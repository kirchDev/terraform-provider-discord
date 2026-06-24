# Convert a hex color into the decimal RGB integer Discord uses.
data "discord_color" "blurple" {
  hex = "#5865F2"
}

# Or provide an RGB triple instead of hex.
data "discord_color" "white" {
  rgb = [255, 255, 255]
}

# Use data.discord_color.blurple.dec for discord_role.color.
