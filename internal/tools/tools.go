package tools

import (
	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/siu"
	siutools "github.com/111hell/tinkerbot/internal/tools/siu"
)

type definition struct {
	name string
	new  func(*siu.Client) agent.Tool
}

var siuTools = []definition{
	{name: siutools.ListMessagesName, new: siutools.NewListMessages},
	{name: siutools.SearchContactsName, new: siutools.NewSearchContacts},
}

func DefaultSIUNames() []string {
	names := make([]string, 0, len(siuTools))
	for _, tool := range siuTools {
		names = append(names, tool.name)
	}
	return names
}

func IsKnown(name string) bool {
	for _, tool := range siuTools {
		if tool.name == name {
			return true
		}
	}
	return false
}

// RequiresSIU reports whether any of the selected tools belong to SIU.
func RequiresSIU(names []string) bool {
	for _, name := range names {
		for _, tool := range siuTools {
			if tool.name == name {
				return true
			}
		}
	}
	return false
}

func All(client *siu.Client) []agent.Tool {
	tools := make([]agent.Tool, 0, len(siuTools))
	for _, tool := range siuTools {
		tools = append(tools, tool.new(client))
	}
	return tools
}
