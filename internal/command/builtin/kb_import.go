package builtin

import (
	"github.com/openscholar/openscholar/internal/command"
)

type kbImportCmd struct{}

func (c *kbImportCmd) Name() string        { return "kb-import" }
func (c *kbImportCmd) Description() string { return "Batch import documents into the knowledge base" }

func (c *kbImportCmd) Execute(ctx command.Context) command.Result {
	if ctx.Args == "" {
		return command.Result{
			Output: "Usage:\n  /kb-import <directory_or_files>  Import documents into KB\n  /kb-status                      Show import progress\n  /kb-retry                       Retry failed imports",
		}
	}

	return command.Result{
		Prompt: "Batch import documents from: " + ctx.Args + "\n\nUse the Bash tool to list supported files (.pdf, .docx, .pptx, .xlsx) in the given path, then use KBAdd to import each one. Report progress after each import.",
	}
}

type kbStatusCmd struct{}

func (c *kbStatusCmd) Name() string        { return "kb-status" }
func (c *kbStatusCmd) Description() string { return "Show batch import job status" }

func (c *kbStatusCmd) Execute(ctx command.Context) command.Result {
	return command.Result{
		Prompt: "Show the current batch import status. List pending, processing, completed, and failed jobs with their file paths.",
	}
}

type kbRetryCmd struct{}

func (c *kbRetryCmd) Name() string        { return "kb-retry" }
func (c *kbRetryCmd) Description() string { return "Retry failed batch import jobs" }

func (c *kbRetryCmd) Execute(ctx command.Context) command.Result {
	return command.Result{
		Prompt: "Retry all failed batch import jobs. Reset their status to pending and re-process them.",
	}
}
