package main

import (
	"fmt"
	"github.com/c-bata/go-prompt"
	"log"
	"regexp"
	"rolld/ctrl"
	"rolld/internal"
	"strings"
)

var (
	space  = regexp.MustCompile(" +")
	prefix = regexp.MustCompile("^(up|rollback|prune) .*?")
)

func main() {
	instance := ctrl.Init()
	if instance == nil {
		log.Println("init error")
		return
	}
	fmt.Println("hello, developer !")
	for {
		input := prompt.Input("> ", completer, prompt.OptionHistory(nil))
		cmds := space.Split(input, -1)
		if strings.TrimSpace(input) == "exit" {
			fmt.Println("bye !")
			return
		} else if prefix.MatchString(input) {
			if len(cmds) < 2 {
				fmt.Println("please input service name")
				continue
			}
			srv := cmds[1]
			switch cmds[0] {
			case "up":
				fmt.Println("prepare to up new service", srv)
				instance.StartUp(cmds[1])
			case "rollback":
				fmt.Println("prepare to rollback old service", srv)
				instance.Rollback(cmds[1])
			case "prune":
				fmt.Println("prepare to prune old container")
				instance.Prune(cmds[1])
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
		for _, srv := range internal.C.UpstreamServer {
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
