# file_data_uri is a write-only base64 data URI; load it from a local file.
data "discord_local_file" "sticker" {
  path = "sticker.png"
}

resource "discord_sticker" "example" {
  server_id     = "123456789012345678"
  name          = "wave"
  description   = "A friendly wave"
  tags          = "wave"
  file_data_uri = data.discord_local_file.sticker.data_uri
}
