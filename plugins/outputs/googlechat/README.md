# Google Chat Webhook Output Plugin

The `googlechat` output plugin sends messages to Google Chat rooms using webhooks. The plugin supports both regular messages and threaded messages.

## Features

- Send messages to Google Chat using webhooks
- Support for threaded messages
- Configurable message batching
- Retry mechanism for failed messages
- TLS configuration support

## Configuration

```toml
[[outputs]]
  [outputs.googlechat]
    # Google Chat webhook URL (required)
    webhook_url = "https://chat.googleapis.com/v1/spaces/SPACE_ID/messages?key=KEY&token=TOKEN"

    # Field to extract the thread message content from (optional)
    # If specified, content of this field will be added as a separate message
    thread_message_field = "thread_message"

    # Enable default threading using event ID
    default_thread_enabled = false

    # Batch events configuration
    batch_buffer = 10      # events buffer size
    batch_interval = "5s"  # interval between event buffer flushes

    # HTTP client configuration
    timeout = "10s"             # request timeout
    idle_conn_timeout = "1m"    # idle connection timeout
    max_idle_conns = 10         # maximum idle connections
    idle_timeout = "1h"         # idle timeout for inactive requesters

    # Retry configuration
    retry_attempts = 3          # maximum retry attempts
    retry_after = "5s"          # interval between retries

    ## TLS configuration
    # if true, TLS client will be used
    tls_enable = false
    # trusted root certificates for server
    tls_ca_file = "/etc/neptunus/ca.pem"
    # used for TLS client certificate authentication
    tls_key_file = "/etc/neptunus/key.pem"
    tls_cert_file = "/etc/neptunus/cert.pem"
    # minimum TLS version, not limited by default
    tls_min_version = "TLS12"
    # send the specified TLS server name via SNI
    tls_server_name = "chat.googleapis.com"
    # use TLS but skip chain & host verification
    tls_insecure_skip_verify = false
```

## Example Usage

### Basic Message

To send a simple message to Google Chat:

```toml
[[outputs]]
  [outputs.googlechat]
    webhook_url = "https://chat.googleapis.com/v1/spaces/SPACE_ID/messages?key=KEY&token=TOKEN"
```

### Threaded Messages

To send messages with thread support:

```toml
[[outputs]]
  [outputs.googlechat]
    webhook_url = "https://chat.googleapis.com/v1/spaces/SPACE_ID/messages?key=KEY&token=TOKEN"
    default_thread_enabled = true
```

### Adding Thread Messages

To include additional message content in threads:

```toml
[[outputs]]
  [outputs.googlechat]
    webhook_url = "https://chat.googleapis.com/v1/spaces/SPACE_ID/messages?key=KEY&token=TOKEN"
    thread_message_field = "description"
    default_thread_enabled = true
```

## How it works

The plugin serializes each event into a message and sends it to the Google Chat webhook URL. If `thread_message_field` is specified, an additional message will be sent with the content of that field. When `default_thread_enabled` is true, messages will be grouped into threads using the event ID as the thread key.

The plugin automatically adds the `messageReplyOption=REPLY_MESSAGE_FALLBACK_TO_NEW_THREAD` parameter to the webhook URL to ensure proper threading behavior.
