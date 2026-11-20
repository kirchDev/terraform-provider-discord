package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// forumPostCfg renders a discord_forum_post in the seeded forum "500" with the
// given content, written as an HCL heredoc so it ends in a newline.
func forumPostCfg(content string) string {
	return `
resource "discord_forum_post" "test" {
  channel_id = "500"
  name       = "Rules"
  content    = <<-EOT
    ` + content + `
  EOT
}
`
}

// TestAccForumPostResourceTrailingNewline is issue #81: Discord strips the
// trailing newline a heredoc leaves on the content, so the read-back after create
// and after update must not be reported as an inconsistent result, and the next
// plan must be empty.
func TestAccForumPostResourceTrailingNewline(t *testing.T) {
	m := newMockDiscord(t)
	m.seedForumChannel("500", "999", "rules")
	const rn = "discord_forum_post.test"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: forumPostCfg("Mind the trailing newline."),
				Check:  resource.TestCheckResourceAttr(rn, "content", "Mind the trailing newline.\n"),
			},
			{
				Config: forumPostCfg("Edited, still with a trailing newline."),
				Check:  resource.TestCheckResourceAttr(rn, "content", "Edited, still with a trailing newline.\n"),
			},
		},
	})
}

func TestKeepContent(t *testing.T) {
	cases := []struct {
		name    string
		current types.String
		api     string
		want    types.String
	}{
		{"trailing newline stripped by Discord", types.StringValue("hello\n"), "hello", types.StringValue("hello\n")},
		{"surrounding whitespace stripped", types.StringValue("  hello \n"), "hello", types.StringValue("  hello \n")},
		{"identical", types.StringValue("hello"), "hello", types.StringValue("hello")},
		{"real drift", types.StringValue("hello\n"), "edited", types.StringValue("edited")},
		{"inner whitespace differs", types.StringValue("a  b"), "a b", types.StringValue("a b")},
		{"import (null)", types.StringNull(), "hello", types.StringValue("hello")},
		{"unknown", types.StringUnknown(), "hello", types.StringValue("hello")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := keepContent(c.current, c.api); !got.Equal(c.want) {
				t.Errorf("keepContent(%s, %q) = %s, want %s", c.current, c.api, got, c.want)
			}
		})
	}
}
