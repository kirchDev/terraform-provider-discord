# Read a local image file and expose it as a base64 data URI,
# ready to feed into icon_data_uri / image_data_uri / avatar_data_uri.
data "discord_local_image" "icon" {
  path = "${path.module}/icon.png"
}

# e.g. image_data_uri = data.discord_local_image.icon.data_uri
