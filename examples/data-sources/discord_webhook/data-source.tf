data "discord_webhook" "deploys" {
  webhook_id = "123456789012345678"
}

output "deploy_webhook_url" {
  value     = data.discord_webhook.deploys.url
  sensitive = true
}
