# The "Server Rules" gate of a Community guild: new members must agree before
# they can chat. The rules are the `values` of a single `TERMS` field.
resource "discord_member_verification" "main" {
  server_id   = "123456789012345678"
  enabled     = true
  description = "Welcome to the server! Please read and agree to the rules."

  form_fields = [
    {
      field_type = "TERMS" # the only type Discord supports
      label      = "Read and agree to the server rules"
      required   = true
      values = [
        "Treat everyone with respect. No harassment, hate speech or discrimination.",
        "No spam or self-promotion without permission.",
        "Keep content safe for work — no NSFW or graphic material.",
        "If you see something against the rules, notify the staff.",
      ]
    },
  ]
}
