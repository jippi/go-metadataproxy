package main

import (
	"github.com/jippi/go-metadataproxy/internal"
)

func main() {
	internal.ConfigureDocker()
	internal.ConfigureAWS()
	internal.StarServer()
}
