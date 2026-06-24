# image_data_uri is a write-only base64 data URI; load it from a local file.
data "discord_local_file" "logo" {
  path = "logo.png"
}

resource "discord_application_emoji" "logo" {
  application_id = "123456789012345678"
  name           = "company_logo"
  image_data_uri = data.discord_local_file.logo.data_uri
}
