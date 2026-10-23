package ggs

import (
	"fmt"
	"strings"
	"time"
)

// defaultVersion is assumed when a script omits the version header.
const defaultVersion = "1.0"

func Parse(input string) (*Guard, error) {
	guard := &Guard{}
	lines := strings.Split(input, "\n")

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		verb, args := splitLine(line)
		switch verb {
		case "guard":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: guard requires a name", i+1)
			}
			guard.Name = args[0]

		case "version":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: version requires a value", i+1)
			}
			if err := checkVersion(args[0]); err != nil {
				return nil, fmt.Errorf("line %d: %w", i+1, err)
			}
			guard.Version = args[0]

		case "os":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: os requires a value", i+1)
			}
			guard.OS = args[0]

		case "watch":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: watch requires a path", i+1)
			}
			guard.Watch = args[0]

		case "debounce":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: debounce requires a duration", i+1)
			}
			d, err := time.ParseDuration(args[0])
			if err != nil {
				return nil, fmt.Errorf("line %d: invalid debounce duration %q: %w", i+1, args[0], err)
			}
			guard.Debounce = d

		case "zip":
			guard.Commands = append(guard.Commands, Command{
				Verb: "zip",
				Args: args,
			})

		case "sync":
			guard.Commands = append(guard.Commands, Command{
				Verb: "sync",
				Args: args,
			})

		case "message":
			msg := strings.Join(args, " ")
			guard.Commands = append(guard.Commands, Command{
				Verb: "message",
				Args: []string{msg},
			})

		case "notify":
			msg := strings.Join(args, " ")
			guard.Commands = append(guard.Commands, Command{
				Verb: "notify",
				Args: []string{msg},
			})

		case "run":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: run requires a command", i+1)
			}
			guard.Commands = append(guard.Commands, Command{
				Verb: "run",
				Args: args,
			})

		case "delete":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: delete requires a path", i+1)
			}
			guard.Commands = append(guard.Commands, Command{
				Verb: "delete",
				Args: args,
			})

		case "webhook":
			if len(args) == 0 {
				return nil, fmt.Errorf("line %d: webhook requires a URL", i+1)
			}
			guard.Commands = append(guard.Commands, Command{
				Verb: "webhook",
				Args: args,
			})

		default:
			return nil, fmt.Errorf("line %d: unknown command %q", i+1, verb)
		}
	}

	if guard.Name == "" {
		return nil, fmt.Errorf("missing required header: guard \"<name>\"")
	}
	if guard.Version == "" {
		guard.Version = defaultVersion
	}

	return guard, nil
}

// checkVersion accepts any 1.x script version and rejects anything else.
func checkVersion(v string) error {
	major := v
	if i := strings.IndexByte(v, '.'); i >= 0 {
		major = v[:i]
	}
	if major != "1" {
		return fmt.Errorf("unsupported script version %q (this build supports 1.x)", v)
	}
	return nil
}

func splitLine(line string) (verb string, args []string) {
	tokens := tokenize(line)
	if len(tokens) == 0 {
		return "", nil
	}
	return tokens[0], tokens[1:]
}

func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]

		if escaped {
			current.WriteByte(ch)
			escaped = false
			continue
		}

		if ch == '\\' {
			escaped = true
			continue
		}

		if ch == '"' {
			inQuotes = !inQuotes
			continue
		}

		if !inQuotes && ch == ' ' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}