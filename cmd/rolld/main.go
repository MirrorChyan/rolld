package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"rolld/internal"
	"rolld/internal/model"
	"rolld/internal/utils"
	"strings"

	"github.com/c-bata/go-prompt"
)

var (
	space  = regexp.MustCompile(" +")
	prefix = regexp.MustCompile("^(up|rollback|prune) .*?")
)

func main() {
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
			srv := cmds[1]
			var err error
			switch cmds[0] {
			case "up":
				fmt.Println("prepare to up new service", srv)
				err = instance.StartUp(srv)
			case "rollback":
				fmt.Println("prepare to rollback old service", srv)
				err = instance.Rollback(srv)
			case "prune":
				fmt.Println("prepare to prune old container")
				err = instance.Prune(srv)
			}
			if err != nil {
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
		{Text: "up", Description: "start up all service"},
		{Text: "rollback", Description: "rollback to old service"},
		{Text: "prune", Description: "prune unused container"},
		{Text: "exit", Description: "say goodbye"},
	}, d.GetWordBeforeCursor(), true)
}
