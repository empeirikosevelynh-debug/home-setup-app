package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"golden-gate-setup/internal/buildinfo"
	"golden-gate-setup/internal/domain"
	"golden-gate-setup/internal/inspect"
	"golden-gate-setup/internal/plan"
	"golden-gate-setup/internal/ui"
	"io"
	"os"
	"os/signal"
)

func runWith(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, probe func(context.Context, domain.Options) (domain.Host, error)) int {
	flags := flag.NewFlagSet("golden-setup", flag.ContinueOnError)
	flags.SetOutput(errOut)
	preview := flags.Bool("plan", false, "inspect and print a read-only JSON plan")
	accessible := flags.Bool("accessible", false, "use plain prompts")
	version := flags.Bool("version", false, "show version and build provenance")
	flags.Usage = func() {
		fmt.Fprint(errOut, "Golden Gate Setup — reviewed first-time setup for native Apple-silicon macOS 27.\n\nUsage: golden-setup [--plan | --accessible | --version | --help]\n\nThe default opens a wizard. Review and accept its plan before installation.\n--plan only inspects and prints JSON; it does not install or save sessions.\n\n")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errOut, "Unexpected positional arguments.")
		return 2
	}
	if *version {
		fmt.Fprintln(out, buildinfo.String())
		return 0
	}
	if !*preview {
		services := setupServices(probe)
		var err error
		if *accessible {
			_, err = ui.RunPlain(ctx, services, in, out)
		} else {
			_, err = ui.Run(ctx, services, in, out)
		}
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		return 0
	}

	if probe == nil {
		fmt.Fprintln(errOut, "Inspection service is unavailable.")
		return 1
	}
	options := plan.DefaultOptions()
	host, err := probe(ctx, options)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	p, err := plan.Build(host, options)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(p); err != nil {
		fmt.Fprintln(errOut, err)
		return 1
	}
	if !p.Supported {
		return 2
	}
	return 0
}
func run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	return runWith(ctx, args, in, out, errOut, (inspect.Inspector{}).Read)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
