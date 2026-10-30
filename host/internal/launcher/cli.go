// Package launcher implements the short-lived host launcher, independently of the bridge daemon.
package launcher

import (
	"fmt"
	"strings"
)

type options struct {
	name, deleteDir, piDir, stateDir                     string
	yes, list, bridgeStop, ignoreConfig, help, continued bool
	workspaces                                           []string
	secrets                                              []secret
}

func parse(args []string) (options, error) {
	var o options
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			o.help = true
			return o, nil
		case "-c", "--continue":
			o.continued = true
		case "-y", "--yes":
			o.yes = true
		case "-l", "--ls":
			o.list = true
		case "--secret":
			if i+3 >= len(args) {
				return o, fmt.Errorf("--secret requires ENV_NAME HOST COMMAND")
			}
			s := secret{Env: args[i+1], Host: args[i+2], Command: args[i+3]}
			if err := s.Validate(o.secrets); err != nil {
				return o, err
			}
			o.secrets = append(o.secrets, s)
			i += 3
		case "--ignore-config-workspaces":
			o.ignoreConfig = true
		case "--bridge-stop":
			if i != len(args)-1 {
				return o, fmt.Errorf("--bridge-stop must be used by itself")
			}
			o.bridgeStop = true
		case "--":
			o.workspaces = append(o.workspaces, args[i+1:]...)
			return o, nil
		default:
			flag, value, equals := strings.Cut(arg, "=")
			var dest *string
			message := "requires a directory"
			switch flag {
			case "-d", "--delete":
				dest = &o.deleteDir
			case "--name":
				dest = &o.name
				message = "requires a sandbox name"
			case "--pissbox-pi-dir":
				dest = &o.piDir
			case "--pissbox-state-dir":
				dest = &o.stateDir
			}
			// The legacy parser does not accept -d=DIR.
			if dest != nil && !(flag == "-d" && equals) {
				if !equals {
					i++
					if i < len(args) {
						value = args[i]
					}
					if strings.HasPrefix(value, "-") {
						value = ""
					}
				}
				if value == "" {
					if flag == "-d" {
						flag = arg
					}
					return o, fmt.Errorf("%s %s", flag, message)
				}
				*dest = value
			} else if strings.HasPrefix(arg, "-") {
				return o, fmt.Errorf("Unknown option: %s\nUse --help for usage information", arg)
			} else {
				o.workspaces = append(o.workspaces, arg)
			}
		}
	}
	return o, nil
}

func (o options) piArgs() []string {
	if o.continued {
		return []string{"--", "--continue"}
	}
	return nil
}
