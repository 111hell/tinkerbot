package tools

import (
	"github.com/111hell/tinker/agent"

	"github.com/111hell/tinkerbot/internal/siu"
	siutools "github.com/111hell/tinkerbot/internal/tools/siu"
)

type definition struct {
	name        string
	new         func(*siu.Client) agent.Tool
	privateOnly bool
}

var siuTools = []definition{
	{name: siutools.ListMessagesName, new: siutools.NewListMessages, privateOnly: true},
	{name: siutools.SearchContactsName, new: siutools.NewSearchContacts, privateOnly: true},
	{name: siutools.SearchChatsName, new: siutools.NewSearchChats, privateOnly: true},
	{name: siutools.QueryUserName, new: siutools.NewQueryUser},
	{name: siutools.SendPhotoName, new: siutools.NewSendPhoto},
	{name: siutools.EditMessageName, new: siutools.NewEditMessage},
	{name: siutools.RevokeMessageName, new: siutools.NewRevokeMessage},
	{name: siutools.SendEventName, new: siutools.NewSendEvent, privateOnly: true},
}

// ForSIUGroup hides private-chat tools from the model. Each tool also checks
// its trusted context when executed, so filtering alone is not authorization.
func ForSIUGroup(names []string) []string {
	var enabled []string
	for _, name := range names {
		for _, tool := range siuTools {
			if tool.name == name && !tool.privateOnly {
				enabled = append(enabled, name)
			}
		}
	}
	return enabled
}

func DefaultSIUNames() []string {
	return []string{siutools.ListMessagesName, siutools.SearchContactsName, siutools.QueryUserName}
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
