# Matrix-NapCatQQ
A matrix-QQ puppeting bridge based on [NapCatQQ](https://github.com/NapNeko/NapCatQQ) and [mautrix-go](https://github.com/mautrix/go).

## About This Project

This project is building upon [duo/matrix-qq](https://github.com/duo/matrix-qq) and replace the underlying QQ protocol implementation with [NapCatQQ](https://github.com/NapNeko/NapCatQQ).

### This Project's Architecture

**This Project's Architecture:**
```
Matrix ←→ mautrix bridgev2 ←→ OneBot v11 Reverse WebSocket ←→ NapCatQQ ←→ QQ Servers
```

This project no longer features a built-in QQ protocol implementation. Instead, it operates as a **OneBot v11 Reverse WebSocket server**. External NapCatQQ processes connect actively to this service, which then bridges the messages to Matrix.

## Setup & Usage

### 1. Configure matrix-napcatqq (Docker)

First, create a working directory and generate the initial configuration file:

```bash
mkdir -p matrix-napcatqq
# The first run will generate a default config.yaml
docker run --rm -v `pwd`/matrix-napcatqq:/data sevten/matrix-napcatqq
```

Edit the generated `config.yaml` in your `/data/matrix-napcatqq` directory. Ensure you configure the following sections:
- `homeserver`: Set your Matrix homeserver address (e.g., `http://localhost:8008`) and domain.
- `appservice`: Configure the bridge's local listening address and port (e.g., `0.0.0.0:29332`).
- `napcat`: Configure the OneBot reverse WebSocket server where NapCatQQ will connect.
  ```yaml
  napcat:
    listen_address: 0.0.0.0:8080
    websocket_path: /onebot/v11/ws
    access_token: "your_secret_token" # Highly recommended to set a token
  ```

Next, generate the appservice registration file for Synapse by running the container again:

```bash
# The second run will generate registration.yaml based on your config
docker run --rm -v `pwd`/matrix-napcatqq:/data sevten/matrix-napcatqq
```
This command creates a `registration.yaml` file.

### 2. Configure Synapse

Copy the generated `registration.yaml` to your Synapse configuration directory. Then, edit your Synapse `homeserver.yaml` to include the appservice:

```yaml
app_service_config_files:
  - /path/to/your/synapse/registration.yaml
```

Restart Synapse to apply the configuration:
```bash
systemctl restart matrix-synapse
```

### 3. Configure NapCatQQ

matrix-napcatqq listens as a OneBot 11 reverse WebSocket server. Start the bridge first, then configure each NapCatQQ instance to connect to the bridge endpoint.

In your NapCatQQ configuration (e.g., `onebot11.json` or via WebUI), add a reverse WebSocket connection:

```json
{
  "network": {
    "websocketReverses": [
      {
        "url": "ws://<matrix-napcatqq-ip>:8080/onebot/v11/ws",
        "enable": true
      }
    ]
  }
}
```

*Note: Replace `<matrix-napcatqq-ip>` with the actual IP address of the machine running matrix-napcatqq. If you set an `access_token` in step 1, ensure you append it as a header or query parameter in NapCatQQ, depending on how NapCatQQ handles OneBot v11 auth.*

### 4. Start and Bind

1. Start the `matrix-napcatqq` bridge using Docker:
   ```bash
   docker run -d --name matrix-napcatqq \
     -v /data/matrix-napcatqq:/data \
     -p 8080:8080 \
     -p 29332:29332 \
     sevten/matrix-napcatqq
   ```
2. Start `NapCatQQ`. You should see successful connection logs in the `matrix-napcatqq` output (`docker logs matrix-napcatqq`).
3. Open your Matrix client (e.g., Element) and start a direct chat with the bridge management bot (usually `@qqbot:yourdomain.com`).
4. Send the `login` command to the bot and enter your QQ number. This binds the connected NapCatQQ session to your Matrix user.

Multiple NapCatQQ instances can connect to the same bridge endpoint; events and API calls are automatically routed by `self_id`.

### Features & roadmap

* Matrix → QQ
  * [x] Message types
    * [x] Text
    * [x] Image
    * [x] Sticker
    * [x] Video
    * [x] Audio
    * [x] File
    * [x] Mention
    * [x] Reply
    * [x] Location
  * [x] Chat types
	  * [x] Direct
	  * [x] Room
  * [ ] Presence
  * [x] Redaction
  * [ ] Group actions
    * [x] Join
    * [ ] Invite
    * [x] Leave
    * [x] Kick
    * [ ] Mute
  * [ ] Room metadata
    * [x] Name
    * [x] Avatar
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
    * [x] Audio
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
    * [x] Mute
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
