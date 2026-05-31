# matrix-qq
A Matrix-QQ puppeting bridge based on [NapCatQQ](https://github.com/NapNeko/NapCatQQ), [OneBot 11](https://github.com/botuniverse/onebot-11), and [mautrix-go](https://github.com/mautrix/go).

### NapCatQQ

matrix-qq listens as a OneBot 11 reverse WebSocket server. Start the bridge first, then configure each NapCatQQ instance to connect to the bridge endpoint, for example:

```yaml
napcat:
  listen_address: 0.0.0.0:8080
  websocket_path: /onebot/v11/ws
  access_token: ""
  request_timeout: 30
```

NapCatQQ reverse WebSocket URL:

```text
ws://matrix-qq:8080/onebot/v11/ws
```

After NapCatQQ connects, use the bridge login flow and enter the QQ number to bind that connected account to your Matrix user. Multiple NapCatQQ instances can connect to the same endpoint; events and API calls are routed by `self_id`.

### Documentation

Some quick links:

* [Bridge setup](https://docs.mau.fi/bridges/go/setup.html)
* [Docker](https://hub.docker.com/r/lxduo/matrix-qq)
* [Step by Step (Chinese)](https://duo.github.io/posts/matrix-qq-wechat/)

### Features & roadmap

* Matrix → QQ
  * [ ] Message types
    * [x] Text
    * [x] Image
    * [x] Sticker
    * [x] Video
    * [ ] Audio
    * [ ] File
    * [x] Mention
    * [x] Reply
    * [x] Location
  * [x] Chat types
	  * [x] Direct
	  * [x] Room
  * [ ] Presence
  * [x] Redaction
  * [ ] Group actions
    * [ ] Join
    * [ ] Invite
    * [ ] Leave
    * [ ] Kick
    * [ ] Mute
  * [ ] Room metadata
    * [ ] Name
    * [ ] Avatar
    * [ ] Topic
  * [ ] User metadata
    * [ ] Name
    * [ ] Avatar

* QQ → Matrix
  * [ ] Message types
    * [x] Text
    * [x] Image
    * [ ] Sticker
    * [x] Video
    * [ ] Audio
    * [x] File
    * [x] Mention
    * [x] Reply
    * [x] Location
  * [ ] Chat types
    * [x] Private
    * [x] Group
    * [ ] Stranger (unidirectional)
  * [ ] Presence
  * [x] Redaction
  * [ ] Group actions
    * [ ] Invite
    * [x] Join
    * [x] Leave
    * [x] Kick
    * [ ] Mute
  * [ ] Group metadata
    * [x] Name
    * [x] Avatar
    * [x] Topic
  * [x] User metadata
    * [x] Name
    * [x] Avatar
  * [ ] Login types
	  * [ ] Password
	  * [x] NapCatQQ reverse WebSocket binding

* Misc
  * [ ] Automatic portal creation
    * [ ] After login
    * [ ] When added to group
    * [x] When receiving message
  * [x] Double puppeting
