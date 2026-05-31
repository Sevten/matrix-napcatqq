package msgconv

import (
	"context"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	"github.com/antchfx/xmlquery"
	"github.com/sevten/matrix-napcatqq/pkg/onebot"
	"github.com/sevten/matrix-napcatqq/pkg/qqid"
	"github.com/gabriel-vasile/mimetype"
	"github.com/rs/zerolog"
	"github.com/tidwall/gjson"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

type contextKey int

const (
	contextKeySession contextKey = iota
	contextKeyIntent
	contextKeyPortal
)

func (mc *MessageConverter) ToMatrix(
	ctx context.Context,
	sess *onebot.Session,
	portal *bridgev2.Portal,
	intent bridgev2.MatrixAPI,
	msg *qqid.Message,
) *bridgev2.ConvertedMessage {
	ctx = context.WithValue(ctx, contextKeySession, sess)
	ctx = context.WithValue(ctx, contextKeyIntent, intent)
	ctx = context.WithValue(ctx, contextKeyPortal, portal)

	var part *bridgev2.ConvertedMessagePart
	switch msg.Type {
	case qqid.MsgImage:
		part = mc.convertImageMessage(ctx, msg)
	case qqid.MsgAudio, qqid.MsgVideo, qqid.MsgFile:
		parts := mc.convertMediaMessage(ctx, msg)
		if len(parts) > 0 {
			part = parts[0]
		}
	case qqid.MsgApp:
		part = mc.convertAppMessage(ctx, msg)
	case qqid.MsgRevoke:
		part = mc.convertRevokeMessage(ctx, msg)
	case qqid.MsgLocation:
		part = mc.convertOneBotLocationMessage(msg)
	}
	if part == nil {
		part = mc.convertTextMessage(msg)
	}

	part.Content.Mentions = &event.Mentions{}
	mc.addMentions(ctx, msg.Elements, part.Content)

	cm := &bridgev2.ConvertedMessage{Parts: []*bridgev2.ConvertedMessagePart{part}}
	if msg.Type == qqid.MsgRevoke {
		cm.ReplyTo = &networkid.MessageOptionalPartID{MessageID: qqid.MakeMessageID(msg.ChatID, msg.ID)}
	} else if replyID := findReplyID(msg.Elements); replyID != "" {
		cm.ReplyTo = &networkid.MessageOptionalPartID{MessageID: qqid.MakeMessageID(msg.ChatID, replyID)}
	}
	return cm
}

func (mc *MessageConverter) convertTextMessage(msg *qqid.Message) *bridgev2.ConvertedMessagePart {
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    toContent(msg.Elements),
		},
	}
}

func (mc *MessageConverter) convertImageMessage(ctx context.Context, msg *qqid.Message) *bridgev2.ConvertedMessagePart {
	parts := mc.convertMediaMessage(ctx, msg)
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 0 {
		return mc.convertTextMessage(msg)
	}

	var imagesMarkdown strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&imagesMarkdown, "![%s](%s)\n", part.Content.FileName, part.Content.URL)
	}
	rendered := format.RenderMarkdown(imagesMarkdown.String(), true, false)
	content := toContent(msg.Elements)
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgText,
			Format:        event.FormatHTML,
			Body:          content,
			FormattedBody: fmt.Sprintf("%s\n%s", rendered.FormattedBody, content),
		},
	}
}

func (mc *MessageConverter) convertMediaMessage(ctx context.Context, msg *qqid.Message) []*bridgev2.ConvertedMessagePart {
	parts := make([]*bridgev2.ConvertedMessagePart, 0)
	for _, elem := range msg.Elements {
		if elem.Type == "image" || elem.Type == "record" || elem.Type == "video" || elem.Type == "file" {
			if part, err := mc.reuploadAttachment(ctx, elem); err != nil {
				parts = append(parts, mc.makeMediaFailure(ctx, err))
			} else {
				parts = append(parts, part)
			}
		}
	}
	return parts
}

func (mc *MessageConverter) convertAppMessage(_ context.Context, msg *qqid.Message) *bridgev2.ConvertedMessagePart {
	if len(msg.Elements) == 0 {
		return mc.convertTextMessage(msg)
	}
	elem := msg.Elements[0]
	content := stringValue(elem.Data, "data")
	if content == "" {
		content = stringValue(elem.Data, "content")
	}
	if elem.Type == "xml" {
		body := content
		var summary strings.Builder
		if doc, err := xmlquery.Parse(strings.NewReader(content)); err == nil {
			if action := xmlquery.FindOne(doc, "//msg[@action='viewMultiMsg']"); action != nil {
				for _, title := range xmlquery.Find(doc, "//item/title") {
					fmt.Fprintf(&summary, "%s\n", title.InnerText())
				}
			}
		}
		if summary.Len() > 0 {
			body = summary.String()
		}
		return &bridgev2.ConvertedMessagePart{
			Type:    event.EventMessage,
			Content: &event.MessageEventContent{Body: body, MsgType: event.MsgText},
		}
	}
	if elem.Type == "json" {
		view := gjson.Get(content, "view").String()
		if view == "LocationShare" {
			name := gjson.Get(content, "meta.*.name").String()
			address := gjson.Get(content, "meta.*.address").String()
			latitude := gjson.Get(content, "meta.*.lat").Float()
			longitude := gjson.Get(content, "meta.*.lng").Float()
			return mc.convertLocationMessage(name, address, latitude, longitude)
		}
	}
	return mc.convertTextMessage(msg)
}

func (mc *MessageConverter) convertOneBotLocationMessage(msg *qqid.Message) *bridgev2.ConvertedMessagePart {
	if len(msg.Elements) == 0 {
		return mc.convertTextMessage(msg)
	}
	elem := msg.Elements[0]
	name := stringValue(elem.Data, "title")
	address := stringValue(elem.Data, "content")
	lat := floatValue(elem.Data, "lat")
	lng := floatValue(elem.Data, "lon")
	return mc.convertLocationMessage(name, address, lat, lng)
}

func (mc *MessageConverter) convertLocationMessage(name, address string, lat, lng float64) *bridgev2.ConvertedMessagePart {
	url := fmt.Sprintf("https://maps.google.com/?q=%.5f,%.5f", lat, lng)
	if len(name) == 0 {
		latChar := 'N'
		if lat < 0 {
			latChar = 'S'
		}
		longChar := 'E'
		if lng < 0 {
			longChar = 'W'
		}
		name = fmt.Sprintf("%.4f deg %c %.4f deg %c", math.Abs(lat), latChar, math.Abs(lng), longChar)
	}
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgLocation,
			Body:          fmt.Sprintf("Location: %s\n%s\n%s", name, address, url),
			Format:        event.FormatHTML,
			FormattedBody: fmt.Sprintf("Location: <a href='%s'>%s</a><br>%s", url, html.EscapeString(name), html.EscapeString(address)),
			GeoURI:        fmt.Sprintf("geo:%.5f,%.5f", lat, lng),
		},
	}
}

func (mc *MessageConverter) convertRevokeMessage(_ context.Context, _ *qqid.Message) *bridgev2.ConvertedMessagePart {
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType:       event.MsgNotice,
			Format:        event.FormatHTML,
			Body:          "revoke message",
			FormattedBody: "<del>revoke message</del>",
		},
	}
}

func (mc *MessageConverter) reuploadAttachment(ctx context.Context, elem onebot.Segment) (*bridgev2.ConvertedMessagePart, error) {
	url := stringValue(elem.Data, "url")
	fileID := stringValue(elem.Data, "file")
	fileName := stringValue(elem.Data, "file_name")
	if fileName == "" {
		fileName = fileID
	}
	if url == "" && fileID != "" {
		if resp, err := getSession(ctx).GetFile(ctx, fileID); err == nil {
			url = resp.URL
			if fileName == "" {
				fileName = resp.File
			}
		}
	}
	if url == "" {
		return nil, fmt.Errorf("OneBot %s segment has no downloadable URL", elem.Type)
	}
	data, err := qqid.GetBytes(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download attachment: %w", err)
	}
	if elem.Type == "record" {
		if converted, err := silk2ogg(data); err == nil {
			data = converted
		}
	}

	mime := mimetype.Detect(data)
	content := &event.MessageEventContent{Info: &event.FileInfo{}}
	switch elem.Type {
	case "image":
		content.MsgType = event.MsgImage
	case "record":
		content.MsgType = event.MsgAudio
		content.MSC3245Voice = &event.MSC3245Voice{}
	case "video":
		content.MsgType = event.MsgVideo
	default:
		content.MsgType = event.MsgFile
	}
	content.Info.Size = len(data)
	content.Info.MimeType = mime.String()
	content.FileName = fileName
	if content.FileName == "" {
		content.FileName = "attachment" + mime.Extension()
	}
	content.URL, content.File, err = getIntent(ctx).UploadMedia(ctx, getPortal(ctx).MXID, data, content.FileName, mime.String())
	if err != nil {
		return nil, err
	}
	return &bridgev2.ConvertedMessagePart{Type: event.EventMessage, Content: content}, nil
}

func (mc *MessageConverter) makeMediaFailure(ctx context.Context, err error) *bridgev2.ConvertedMessagePart {
	zerolog.Ctx(ctx).Err(err).Msg("Failed to reupload QQ attachment")
	return &bridgev2.ConvertedMessagePart{
		Type: event.EventMessage,
		Content: &event.MessageEventContent{
			MsgType: event.MsgNotice,
			Body:    fmt.Sprintf("Failed to upload QQ attachment: %v", err),
		},
	}
}

func (mc *MessageConverter) addMentions(ctx context.Context, elems []onebot.Segment, into *event.MessageEventContent) {
	mentionedID := make([]string, 0)
	for _, elem := range elems {
		if elem.Type != "at" {
			continue
		}
		id := stringValue(elem.Data, "qq")
		if id == "all" {
			mentionedID = append(mentionedID, "room")
		} else if id != "" {
			mentionedID = append(mentionedID, id)
		}
	}
	if len(mentionedID) == 0 {
		return
	}
	into.EnsureHasHTML()
	for _, id := range mentionedID {
		if id == "room" {
			into.Mentions.Room = true
			continue
		}
		mxid, displayname, err := mc.getBasicUserInfo(ctx, qqid.MakeUserID(id))
		if err != nil {
			zerolog.Ctx(ctx).Err(err).Str("id", id).Msg("Failed to get user info")
			continue
		}
		into.Mentions.UserIDs = append(into.Mentions.UserIDs, mxid)
		mentionText := "@" + id
		into.Body = strings.ReplaceAll(into.Body, mentionText, displayname)
		into.FormattedBody = strings.ReplaceAll(into.FormattedBody, mentionText, fmt.Sprintf(`<a href="%s">%s</a>`, mxid.URI().MatrixToURL(), html.EscapeString(displayname)))
	}
}

func (mc *MessageConverter) getBasicUserInfo(ctx context.Context, user networkid.UserID) (id.UserID, string, error) {
	ghost, err := mc.Bridge.GetGhostByID(ctx, user)
	if err != nil {
		return "", "", fmt.Errorf("failed to get ghost by ID: %w", err)
	}
	login := mc.Bridge.GetCachedUserLoginByID(networkid.UserLoginID(user))
	if login != nil {
		return login.UserMXID, ghost.Name, nil
	}
	return ghost.Intent.GetMXID(), ghost.Name, nil
}

func toContent(elems []onebot.Segment) string {
	var content strings.Builder
	for _, elem := range elems {
		switch elem.Type {
		case "reply":
		case "text":
			fmt.Fprint(&content, stringValue(elem.Data, "text"))
		case "json", "xml":
			fmt.Fprint(&content, stringValue(elem.Data, "data"))
		case "at":
			target := stringValue(elem.Data, "qq")
			if target == "all" {
				fmt.Fprint(&content, "@room")
			} else if target != "" {
				fmt.Fprintf(&content, "@%s", target)
			}
		case "forward":
			fmt.Fprintf(&content, "[Forward: %s]", stringValue(elem.Data, "id"))
		case "face":
			fmt.Fprintf(&content, "/[Face%s]", stringValue(elem.Data, "id"))
		case "image":
			fmt.Fprintf(&content, "[Image]")
		case "record":
			fmt.Fprintf(&content, "[Voice]")
		case "video":
			fmt.Fprintf(&content, "[Video]")
		case "file":
			fmt.Fprintf(&content, "[File]")
		case "location":
			fmt.Fprintf(&content, "[Location]")
		default:
			fmt.Fprintf(&content, "[%s]", elem.Type)
		}
	}
	return content.String()
}

func findReplyID(elems []onebot.Segment) string {
	for _, elem := range elems {
		if elem.Type == "reply" {
			return stringValue(elem.Data, "id")
		}
	}
	return ""
}

func getSession(ctx context.Context) *onebot.Session {
	return ctx.Value(contextKeySession).(*onebot.Session)
}

func getIntent(ctx context.Context) bridgev2.MatrixAPI {
	return ctx.Value(contextKeyIntent).(bridgev2.MatrixAPI)
}

func getPortal(ctx context.Context) *bridgev2.Portal {
	return ctx.Value(contextKeyPortal).(*bridgev2.Portal)
}

func stringValue(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	val, ok := data[key]
	if !ok || val == nil {
		return ""
	}
	return onebot.AnyID(val).String()
}

func floatValue(data map[string]any, key string) float64 {
	if data == nil {
		return 0
	}
	switch val := data[key].(type) {
	case float64:
		return val
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		f, _ := strconv.ParseFloat(fmt.Sprint(val), 64)
		return f
	}
}
