package cmd

import "github.com/vicentereig/whatsapp-cli/cmd/registry"

var reg *registry.Registry

func SetVersion(v string) {
	reg = registry.NewRegistry()
	reg.SetVersion(v)
	registerAll(reg)
}

func Execute() {
	reg.Execute()
}
