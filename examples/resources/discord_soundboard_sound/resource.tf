# sound_data_uri is a write-only base64 data URI; load it from a local file.
data "discord_local_file" "sound" {
  path = "airhorn.mp3"
}

resource "discord_soundboard_sound" "example" {
  server_id      = "123456789012345678"
  name           = "airhorn"
  sound_data_uri = data.discord_local_file.sound.data_uri
  volume         = 0.8
  emoji_name     = "📢"
}
