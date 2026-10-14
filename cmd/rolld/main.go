package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"rolld/internal"
	"rolld/internal/model"
	"rolld/internal/utils"
	"slices"
	"strings"

	"github.com/c-bata/go-prompt"
)

var (
	space  = regexp.MustCompile(" +")
	prefix = regexp.MustCompile("^(up|rollback|prune) .*?")
)

var commands = []string{"up", "rollback", "prune"}

const usage = `Usage:
  rolld                   start the interactive prompt
  rolld up <srv>          start a new container of srv and switch its upstream to it
  rolld rollback <srv>    switch srv back to its previous container
  rolld prune <srv|all>   remove old containers of srv
`

func main() {
	if len(os.Args) > 1 {
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
	}
	repl()
}

// run executes one command given on the command line and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	switch {
	case len(args) == 1 && slices.Contains([]string{"help", "-h", "--help"}, args[0]):
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	case len(args) != 2 || !slices.Contains(commands, args[0]):
		_, _ = fmt.Fprintf(stderr, "unknown command: %s\n\n%s", strings.Join(args, " "), usage)
		return 2
	}

	instance, err := internal.Init()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	if err := dispatch(instance, args[0], args[1]); err != nil {
		_, _ = fmt.Fprintln(stderr, "error:", err)
		return 1
	}
	return 0
}

func repl() {
	defer func() {
		utils.Restore()
	}()
	instance, err := internal.Init()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("hello, developer !")

	p := prompt.New(
		executor(instance),
		completer,
		prompt.OptionPrefix("> "),
		prompt.OptionHistory(nil),
	)
	p.Run()
}

func dispatch(instance *internal.Instance, cmd, srv string) error {
	switch cmd {
	case "up":
		fmt.Println("prepare to up new service", srv)
		return instance.StartUp(srv)
	case "rollback":
		fmt.Println("prepare to rollback old service", srv)
		return instance.Rollback(srv)
	case "prune":
		fmt.Println("prepare to prune old container")
		return instance.Prune(srv)
	}
	return fmt.Errorf("unknown command %s", cmd)
}

func executor(instance *internal.Instance) func(input string) {
	return func(input string) {
		cmds := space.Split(input, -1)
		if strings.TrimSpace(input) == "exit" {
			fmt.Println("bye !")
			utils.Restore()
			os.Exit(0)
		} else if prefix.MatchString(input) {
			if len(cmds) < 2 {
				fmt.Println("please input service name")
				return
			}
			// pick up edits to nodes.yaml / compose.yaml made since the last command
			if err := instance.Reload(); err != nil {
				fmt.Println("reload config error:", err)
				return
			}
			if err := dispatch(instance, cmds[0], cmds[1]); err != nil {
				fmt.Println("error:", err)
			}
		} else {
			fmt.Println("unknown command", input)
		}
	}
}

func completer(d prompt.Document) []prompt.Suggest {
	match := prefix.FindStringSubmatch(d.Text)
	if len(match) > 0 {
		var s []prompt.Suggest
		if match[1] == "prune" {
			s = append(s, prompt.Suggest{Text: "all"})
		}
		for _, srv := range model.C.UpstreamServer {
			s = append(s, prompt.Suggest{Text: srv.Srv})
		}

		return prompt.FilterHasPrefix(s, d.GetWordBeforeCursor(), false)
	}

	return prompt.FilterHasPrefix([]prompt.Suggest{
		{Text: "up", Description: "start up a new container of a service"},
		{Text: "rollback", Description: "rollback to old service"},
		{Text: "prune", Description: "prune unused container"},
		{Text: "exit", Description: "say goodbye"},
	}, d.GetWordBeforeCursor(), true)
}
