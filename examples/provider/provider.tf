terraform {
  required_providers {
    discord = {
      source = "kirchdev/discord"
    }
  }
}

variable "discord_token" {
  type      = string
  sensitive = true
}

provider "discord" {
  token = var.discord_token
}
