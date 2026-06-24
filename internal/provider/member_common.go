package provider

// memberAttributes mirrors the fields of a Discord guild member object the
// provider maps. The roles array lists role ids and excludes the implicit
// @everyone role (which Discord never returns here).
type memberAttributes struct {
	Nick  *string  `json:"nick"`
	Roles []string `json:"roles"`
	User  struct {
		ID            string  `json:"id"`
		Username      string  `json:"username"`
		GlobalName    *string `json:"global_name"`
		Discriminator string  `json:"discriminator"`
	} `json:"user"`
}

// memberPath is the read/update endpoint for a single guild member.
func memberPath(guildID, userID string) string {
	return "/guilds/" + guildID + "/members/" + userID
}
