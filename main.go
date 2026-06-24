package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/kirchDev/terraform-provider-discord/internal/provider"
)

// Generate the provider documentation under docs/. Pre-publish, tfplugindocs
// can't pull the schema from the registry, so this runs scripts/gen-docs.sh
// (build + local schema export + tfplugindocs). Same as `make docs`.
//go:generate bash scripts/gen-docs.sh

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		// OpenTofu registry address — see CLAUDE.md.
		Address: "registry.opentofu.org/kirchdev/discord",
		Debug:   debug,
	}

	if err := providerserver.Serve(context.Background(), provider.New(version), opts); err != nil {
		log.Fatal(err.Error())
	}
}
