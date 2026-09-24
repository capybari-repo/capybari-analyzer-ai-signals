// Command capybari-ai-signals runs this capability on its own.
package main

import (
	aisignals "github.com/capybari/capybari-analyzer-ai-signals"
	"github.com/capybari/capybari-core/standalone"
)

var version = "dev"

func main() { standalone.Main(version, aisignals.New()) }
