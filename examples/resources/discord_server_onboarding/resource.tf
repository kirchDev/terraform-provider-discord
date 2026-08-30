# Onboarding with one multiple-choice prompt offering two opt-in options.
#
# Every prompt and option carries a `key` you choose. Discord never sees it: it
# is what lets the provider keep the id Discord assigned when you rename a
# prompt, reorder the list, or insert another one above it. Pick keys you will
# not want to change — changing one retires that prompt or option and creates a
# new one with a fresh id, which drops the members' selections hanging on it.
resource "discord_server_onboarding" "main" {
  server_id           = "123456789012345678"
  enabled             = true
  mode                = 0
  default_channel_ids = ["456789012345678901"]

  prompts = [
    {
      key           = "why_here"
      type          = 0 # multiple choice
      title         = "What are you here for?"
      single_select = false
      required      = true
      in_onboarding = true

      options = [
        {
          key         = "announcements"
          title       = "Announcements"
          description = "Get notified about the latest news."
          emoji_name  = "📣" # unicode emoji
          channel_ids = ["456789012345678901"]
        },
        {
          key   = "browsing"
          title = "Just browsing"
        },
      ]
    },
  ]
}
