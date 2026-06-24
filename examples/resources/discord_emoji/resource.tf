# image_data_uri is a write-only base64 data URI; load it from a local file.
data "discord_local_image" "logo" {
  path = "${path.module}/logo.png"
}

resource "discord_emoji" "logo" {
  server_id      = "123456789012345678"
  name           = "company_logo"
  image_data_uri = data.discord_local_image.logo.data_uri
}
