package tools

import (
	"github.com/111hell/tinker/agent"

	"myagent/internal/siu"
	siutools "myagent/internal/tools/siu"
)

func All(client *siu.Client) []agent.Tool {
	return []agent.Tool{
		siutools.NewListMessages(client),
		siutools.NewSearchContacts(client),
	}
}
