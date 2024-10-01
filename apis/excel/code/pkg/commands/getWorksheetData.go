package commands

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gptscript-ai/tools/apis/excel/code/pkg/client"
	"github.com/gptscript-ai/tools/apis/excel/code/pkg/global"
	"github.com/gptscript-ai/tools/apis/excel/code/pkg/graph"
)

func GetWorksheetData(ctx context.Context, workbookID, worksheetID string) error {
	c, err := client.NewClient(global.ReadOnlyScopes)
	if err != nil {
		return err
	}

	data, _, err := graph.GetWorksheetData(ctx, c, workbookID, worksheetID)
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	fmt.Println(string(dataBytes))
	return nil
}
