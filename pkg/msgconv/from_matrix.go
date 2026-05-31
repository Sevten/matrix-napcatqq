package msgconv

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"/pkg/onebot"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
	"maunium.net/go/mautrix/id"
)

func (mc *MessageConverter) ToQQ(
	ctx context.Context,
	evt *event.Event,
	content *event.MessageEventContent,
	portal *bridgev2.Portal,
) ([]onebot.Segment, error) {
	ctx = context.WithValue(ctx, contextKeyPortal, portal)

	if evt.Type == event.EventSticker {
		content.MsgType = event.MessageType(event.EventSticker.Type)
	}

	switch content.MsgType {
	case event.MsgText, event.MsgNotice, event.MsgEmote:
		return mc.constructTextMessage(ctx, content), nil
	case event.MessageType(event.EventSticker.Type), event.MsgImage, event.MsgVideo, event.MsgAudio:
		data, err := mc.Bridge.Bot.DownloadMedia(ctx, content.URL, content.File)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
		}
		return mc.constructMediaMessage(content, data), nil
	case event.MsgFile:
		data, err := mc.Bridge.Bot.DownloadMedia(ctx, content.URL, content.File)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", bridgev2.ErrMediaDownloadFailed, err)
		}
		return mc.constructFileMessage(content, data), nil
	case event.MsgLocation:
		lat, lng, err := parseGeoURI(content.GeoURI)
		if err != nil {
			return nil, err
		}
		return mc.constructLocationMessage(content.Body, lat, lng), nil
	default:
		return nil, fmt.Errorf("%w %s", bridgev2.ErrUnsupportedMessageType, content.MsgType)
	}
}

func (mc *MessageConverter) constructTextMessage(ctx context.Context, content *event.MessageEventContent) []onebot.Segment {
	text, mentions := mc.parseText(ctx, content)
	if content.Mentions != nil && content.Mentions.Room {
		mentions = append(mentions, "all")
	}
	if len(mentions) == 0 {
		return []onebot.Segment{onebot.Text(text)}
	}

	keywords := make([]string, len(mentions))
	for i, m := range mentions {
		keywords[i] = "@" + m
	}
	pattern := strings.Join(keywords, "|")
	re := regexp.MustCompile("(?:" + pattern + ")")

	parts := re.Split(text, -1)
	matches := re.FindAllString(text, -1)

	segments := []onebot.Segment{}
	for i := 0; i < len(parts); i++ {
		if parts[i] != "" {
			segments = append(segments, onebot.Text(parts[i]))
		}
		if i < len(matches) {
			match := matches[i]
			if slices.Contains(keywords, match) {
				if match == "@all" || match == "@room" {
					segments = append(segments, onebot.At("all"))
				} else {
					segments = append(segments, onebot.At(match[1:]))
				}
			}
		}
	}
	return segments
}

func (mc *MessageConverter) constructMediaMessage(content *event.MessageEventContent, data []byte) []onebot.Segment {
	file := "base64://" + base64.StdEncoding.EncodeToString(data)
	switch content.MsgType {
	case event.MessageType(event.EventSticker.Type), event.MsgImage:
		return []onebot.Segment{onebot.NewSegment("image", map[string]any{"file": file})}
	case event.MsgVideo:
		return []onebot.Segment{onebot.NewSegment("video", map[string]any{"file": file})}
	case event.MsgAudio:
		if silk, err := ogg2silk(data); err == nil {
			file = "base64://" + base64.StdEncoding.EncodeToString(silk)
		}
		return []onebot.Segment{onebot.NewSegment("record", map[string]any{"file": file})}
	}
	return nil
}

func (mc *MessageConverter) constructFileMessage(content *event.MessageEventContent, data []byte) []onebot.Segment {
	name := content.Body
	if content.FileName != "" {
		name = content.FileName
	}
	return []onebot.Segment{onebot.NewSegment("file", map[string]any{
		"file": "base64://" + base64.StdEncoding.EncodeToString(data),
		"name": name,
	})}
}

func (mc *MessageConverter) constructLocationMessage(name string, lat, lng float64) []onebot.Segment {
	return []onebot.Segment{onebot.NewSegment("location", map[string]any{
		"lat":     fmt.Sprintf("%.5f", lat),
		"lon":     fmt.Sprintf("%.5f", lng),
		"title":   name,
		"content": name,
	})}
}

func (mc *MessageConverter) parseText(ctx context.Context, content *event.MessageEventContent) (text string, mentions []string) {
	mentions = make([]string, 0)
	parseCtx := format.NewContext(ctx)
	parseCtx.ReturnData["allowed_mentions"] = content.Mentions
	parseCtx.ReturnData["output_mentions"] = &mentions
	if content.Format == event.FormatHTML {
		text = mc.HTMLParser.Parse(content.FormattedBody, parseCtx)
	} else {
		text = content.Body
	}
	return
}

func (mc *MessageConverter) convertPill(displayname, mxid, eventID string, ctx format.Context) string {
	if len(mxid) == 0 || mxid[0] != '@' {
		return format.DefaultPillConverter(displayname, mxid, eventID, ctx)
	}
	allowedMentions, _ := ctx.ReturnData["allowed_mentions"].(*event.Mentions)
	if allowedMentions != nil && !allowedMentions.Has(id.UserID(mxid)) {
		return displayname
	}
	var oid string
	ghost, err := mc.Bridge.GetGhostByMXID(ctx.Ctx, id.UserID(mxid))
	if err != nil {
		return displayname
	} else if ghost != nil {
		oid = string(ghost.ID)
	} else if user, err := mc.Bridge.GetExistingUserByMXID(ctx.Ctx, id.UserID(mxid)); err != nil {
		return displayname
	} else if user != nil {
		portal := getPortal(ctx.Ctx)
		login, _, _ := portal.FindPreferredLogin(ctx.Ctx, user, false)
		if login == nil {
			return displayname
		}
		oid = string(login.ID)
	} else {
		return displayname
	}
	mentions := ctx.ReturnData["output_mentions"].(*[]string)
	*mentions = append(*mentions, oid)
	return fmt.Sprintf("@%s", oid)
}

func parseGeoURI(uri string) (lat, lng float64, err error) {
	if !strings.HasPrefix(uri, "geo:") {
		err = fmt.Errorf("uri doesn't have geo: prefix")
		return
	}
	coordinates := strings.Split(strings.TrimPrefix(uri, "geo:"), ";")[0]
	if splitCoordinates := strings.Split(coordinates, ","); len(splitCoordinates) != 2 {
		err = fmt.Errorf("didn't find exactly two numbers separated by a comma")
	} else if lat, err = strconv.ParseFloat(splitCoordinates[0], 64); err != nil {
		err = fmt.Errorf("latitude is not a number: %w", err)
	} else if lng, err = strconv.ParseFloat(splitCoordinates[1], 64); err != nil {
		err = fmt.Errorf("longitude is not a number: %w", err)
	}
	return
}
