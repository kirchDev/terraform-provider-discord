# Onboarding with one multiple-choice prompt offering two opt-in options.
resource "discord_server_onboarding" "main" {
  server_id           = "123456789012345678"
  enabled             = true
  mode                = 0
  default_channel_ids = ["456789012345678901"]

  prompts = [
    {
      type          = 0 # multiple choice
      title         = "What are you here for?"
      single_select = false
      required      = true
      in_onboarding = true

      options = [
        {
          title       = "Announcements"
          description = "Get notified about the latest news."
          emoji_name  = "📣" # unicode emoji
          channel_ids = ["456789012345678901"]
        },
        {
          title = "Just browsing"
        },
      ]
    },
  ]
}
