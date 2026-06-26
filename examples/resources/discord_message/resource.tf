# A pinned-style rules embed. Author it as a specific bot by pointing the resource
# at an aliased provider (a bot can only edit its own messages).
resource "discord_message" "rules" {
  channel_id = "123456789012345678"

  embeds = [
    {
      title       = "Server Rules"
      description = "Please read and follow the rules below."
      color       = 5793266 # blurple, as a decimal integer
      timestamp   = "2026-06-26T14:48:00+02:00"
      footer_text = "Last updated"

      fields = [
        { name = "1. Respect", value = "Treat everyone with respect." },
        { name = "2. No spam", value = "No unsolicited ads or self-promotion." },
      ]
    },
  ]
}

# Authoring as a specific bot via an aliased provider:
#
#   resource "discord_message" "rules" {
#     provider   = discord.community
#     channel_id = "123456789012345678"
#     content    = "Welcome!"
#   }
