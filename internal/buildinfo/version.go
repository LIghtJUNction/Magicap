// Package buildinfo contains the version supplied from kam.toml at build time.
package buildinfo

// Version is "dev" for ordinary go builds and is set by the Kam build hook.
var Version = "dev"
