package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

func (c *CLI) runTask(ctx context.Context, opts runtimeOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(opts.args) < 2 {
		return &UsageError{
			Message: fmt.Sprintf("missing task command\n\nRun '%s help' for usage.", commandName),
			Code:    2,
		}
	}

	_, _, store, err := c.runtimeStore(ctx, opts)
	if err != nil {
		return err
	}
	defer store.Close()

	switch opts.args[1] {
	case "create":
		return c.runTaskCreate(ctx, store, opts.args[2:])
	case "source":
		return c.runTaskSource(ctx, store, opts.args[2:])
	case "list":
		return c.runTaskList(ctx, store, opts.args[2:])
	case "show":
		return c.runTaskShow(ctx, store, opts.args[2:])
	case "status":
		return c.runTaskStatus(ctx, store, opts.args[2:])
	case "done":
		return c.runTaskDone(ctx, store, opts.args[2:])
	case "comment":
		return c.runTaskComment(ctx, store, opts.args[2:])
	case "progress":
		return c.runTaskProgress(ctx, store, opts.args[2:])
	case "block":
		return c.runTaskBlock(ctx, store, opts.args[2:])
	case "unblock":
		return c.runTaskUnblock(ctx, store, opts.args[2:])
	case "dependency", "dep":
		return c.runTaskDependency(ctx, store, opts.args[2:])
	case "ready":
		return c.runTaskReady(ctx, store, opts.args[2:])
	case "claim":
		return c.runTaskClaim(ctx, store, opts.args[2:])
	default:
		return &UsageError{
			Message: fmt.Sprintf("unknown task command %q\n\nRun '%s help' for usage.", opts.args[1], commandName),
			Code:    2,
		}
	}
}

type taskCreateOptions struct {
	projectName        string
	title              string
	description        string
	acceptanceCriteria string
	notes              string
	source             string
	externalID         string
	externalURL        string
	externalRevision   string
	json               bool
}

type taskListOptions struct {
	projectName string
	status      string
	json        bool
}

type taskStatusOptions struct {
	taskID int64
	status string
	json   bool
}

type taskSourceOptions struct {
	taskID           int64
	source           string
	externalID       string
	externalURL      string
	externalRevision string
	json             bool
}

func (c *CLI) runTaskCreate(ctx context.Context, store *storage.Store, args []string) error {
	createOpts, err := parseTaskCreateOptions(args)
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, createOpts.projectName)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := store.CreateTask(ctx, storage.CreateTaskInput{
		ProjectID:          project.ID,
		Title:              createOpts.title,
		Description:        createOpts.description,
		AcceptanceCriteria: createOpts.acceptanceCriteria,
		Notes:              createOpts.notes,
		Source:             createOpts.source,
		ExternalID:         createOpts.externalID,
		ExternalURL:        createOpts.externalURL,
		ExternalRevision:   createOpts.externalRevision,
		Actor:              actor,
	})
	if err != nil {
		return err
	}

	if createOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

func (c *CLI) runTaskSource(ctx context.Context, store *storage.Store, args []string) error {
	sourceOpts, err := parseTaskSourceOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := store.UpdateTaskExternalReference(ctx, storage.UpdateTaskExternalReferenceInput{
		ID:               sourceOpts.taskID,
		Source:           sourceOpts.source,
		ExternalID:       sourceOpts.externalID,
		ExternalURL:      sourceOpts.externalURL,
		ExternalRevision: sourceOpts.externalRevision,
		Actor:            actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", sourceOpts.taskID)
		}
		return err
	}

	if sourceOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

func (c *CLI) runTaskList(ctx context.Context, store *storage.Store, args []string) error {
	listOpts, err := parseTaskListOptions(args)
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, listOpts.projectName)
	if err != nil {
		return err
	}

	tasks, err := store.ListTasksWithOptions(ctx, project.ID, storage.ListTasksOptions{
		Status: listOpts.status,
	})
	if err != nil {
		return err
	}

	if listOpts.json {
		return printTasksJSON(c.out, tasks)
	}

	if len(tasks) == 0 {
		fmt.Fprintln(c.out, "no tasks")
		return nil
	}

	rows := [][]string{{"id", "status", "title"}}
	for _, task := range tasks {
		rows = append(rows, []string{strconv.FormatInt(task.ID, 10), task.Status, task.Title})
	}
	return printTerminalTable(c.out, rows)
}

func (c *CLI) runTaskShow(ctx context.Context, store *storage.Store, args []string) error {
	showOpts, err := parseTaskShowOptions(args)
	if err != nil {
		return err
	}

	task, err := store.GetTask(ctx, showOpts.taskID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", showOpts.taskID)
		}
		return err
	}

	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return err
	}

	if showOpts.json {
		return printTaskShowJSON(c.out, task, events)
	}

	printTask(c.out, task)
	printTaskEvents(c.out, events)
	return nil
}

func (c *CLI) runTaskStatus(ctx context.Context, store *storage.Store, args []string) error {
	statusOpts, err := parseTaskStatusOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := tokservice.NewTaskService(store).UpdateStatus(ctx, statusOpts.taskID, statusOpts.status, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", statusOpts.taskID)
		}
		if errors.Is(err, storage.ErrActiveRunExists) {
			return fmt.Errorf("task cannot be marked done while an active run exists")
		}
		if errors.Is(err, tokservice.ErrTaskCompletionEvidenceRequired) {
			return fmt.Errorf("task status done requires a succeeded run with passed validation; use task done with --allow-unvalidated and --override-reason for an audited override")
		}
		return err
	}

	if statusOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

type taskDoneOptions struct {
	taskID           int64
	note             string
	evidenceRunID    int64
	allowUnvalidated bool
	overrideReason   string
	json             bool
}

type taskShowOptions struct {
	taskID int64
	json   bool
}

func (c *CLI) runTaskDone(ctx context.Context, store *storage.Store, args []string) error {
	doneOpts, err := parseTaskDoneOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := tokservice.NewTaskService(store).CompleteTask(ctx, tokservice.CompleteTaskInput{
		ID:               doneOpts.taskID,
		Note:             doneOpts.note,
		EvidenceRunID:    doneOpts.evidenceRunID,
		AllowUnvalidated: doneOpts.allowUnvalidated,
		OverrideReason:   doneOpts.overrideReason,
		Actor:            actor,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", doneOpts.taskID)
		}
		if errors.Is(err, storage.ErrInvalidTaskTransition) {
			return fmt.Errorf("task must be in_progress to complete")
		}
		if errors.Is(err, storage.ErrActiveRunExists) {
			return fmt.Errorf("task cannot be completed while an active run exists")
		}
		if errors.Is(err, tokservice.ErrTaskCompletionEvidenceRequired) {
			return fmt.Errorf("task done requires a succeeded evidence run with passed validation; use --evidence-run or --allow-unvalidated with --override-reason")
		}
		if errors.Is(err, tokservice.ErrOverrideReasonRequired) {
			return fmt.Errorf("task done --allow-unvalidated requires --override-reason")
		}
		return err
	}

	if doneOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

type taskCommentOptions struct {
	taskID int64
	body   string
	json   bool
}

func (c *CLI) runTaskComment(ctx context.Context, store *storage.Store, args []string) error {
	commentOpts, err := parseTaskCommentOptions(args)
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	event, err := store.AddTaskCommentByActor(ctx, commentOpts.taskID, commentOpts.body, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", commentOpts.taskID)
		}
		return err
	}

	if commentOpts.json {
		return printTaskEventJSON(c.out, event)
	}
	printTaskEvent(c.out, event)
	return nil
}

func (c *CLI) runTaskProgress(ctx context.Context, store *storage.Store, args []string) error {
	progressOpts, err := parseTaskNoteOptions(args, "task progress", "--body")
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	event, err := store.AddTaskProgressByActor(ctx, progressOpts.taskID, progressOpts.body, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", progressOpts.taskID)
		}
		return err
	}

	if progressOpts.json {
		return printTaskEventJSON(c.out, event)
	}
	printTaskEvent(c.out, event)
	return nil
}

func (c *CLI) runTaskBlock(ctx context.Context, store *storage.Store, args []string) error {
	blockOpts, err := parseTaskNoteOptions(args, "task block", "--reason")
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := store.BlockTaskByActor(ctx, blockOpts.taskID, blockOpts.body, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", blockOpts.taskID)
		}
		if errors.Is(err, storage.ErrInvalidTaskTransition) {
			return fmt.Errorf("task cannot be blocked from current status")
		}
		return err
	}

	if blockOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

func (c *CLI) runTaskUnblock(ctx context.Context, store *storage.Store, args []string) error {
	unblockOpts, err := parseTaskNoteOptions(args, "task unblock", "--note")
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := store.UnblockTaskByActor(ctx, unblockOpts.taskID, unblockOpts.body, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", unblockOpts.taskID)
		}
		if errors.Is(err, storage.ErrInvalidTaskTransition) {
			return fmt.Errorf("task must be blocked to unblock")
		}
		return err
	}

	if unblockOpts.json {
		return printTaskJSON(c.out, task)
	}
	printTask(c.out, task)
	return nil
}

func (c *CLI) runTaskDependency(ctx context.Context, store *storage.Store, args []string) error {
	if len(args) < 1 {
		return &UsageError{Message: "task dependency requires add or remove", Code: 2}
	}

	dependencyOpts, err := parseTaskDependencyOptions(args[1:])
	if err != nil {
		return err
	}

	switch args[0] {
	case "add":
		dependency, err := store.AddTaskDependency(ctx, dependencyOpts.edgeType, dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID)
		if err != nil {
			return err
		}
		if dependencyOpts.json {
			return printTaskDependencyJSON(c.out, dependency)
		}
		printTaskDependency(c.out, dependency)
		return nil
	case "remove":
		if err := store.RemoveTaskDependency(ctx, dependencyOpts.edgeType, dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("task dependency not found: %d blocks %d", dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID)
			}
			return err
		}
		if dependencyOpts.json {
			return printTaskDependencyRemovedJSON(c.out, dependencyOpts)
		}
		fmt.Fprintf(c.out, "removed dependency: %d blocks %d\n", dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID)
		return nil
	default:
		return &UsageError{Message: fmt.Sprintf("unknown task dependency command %q", args[0]), Code: 2}
	}
}

type taskReadyOptions struct {
	projectName string
	json        bool
}

func (c *CLI) runTaskReady(ctx context.Context, store *storage.Store, args []string) error {
	readyOpts, err := parseTaskReadyOptions(args)
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, readyOpts.projectName)
	if err != nil {
		return err
	}

	tasks, err := store.ListReadyTasks(ctx, project.ID)
	if err != nil {
		return err
	}

	if readyOpts.json {
		return printReadyTasksJSON(c.out, tasks)
	}

	if len(tasks) == 0 {
		fmt.Fprintln(c.out, "no ready tasks")
		return nil
	}

	rows := [][]string{{"id", "status", "title"}}
	for _, task := range tasks {
		rows = append(rows, []string{strconv.FormatInt(task.ID, 10), task.Status, task.Title})
	}
	return printTerminalTable(c.out, rows)
}

type taskClaimOptions struct {
	projectName string
	taskID      int64
	json        bool
}

func (c *CLI) runTaskClaim(ctx context.Context, store *storage.Store, args []string) error {
	claimOpts, err := parseTaskClaimOptions(args)
	if err != nil {
		return err
	}

	project, err := getProjectForTask(ctx, store, claimOpts.projectName)
	if err != nil {
		return err
	}

	var task storage.Task
	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}
	if claimOpts.taskID > 0 {
		task, err = store.ClaimTaskByActor(ctx, project.ID, claimOpts.taskID, actor)
	} else {
		task, err = store.ClaimNextReadyTaskByActor(ctx, project.ID, actor)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", claimOpts.taskID)
		}
		if errors.Is(err, storage.ErrNoReadyTask) {
			return fmt.Errorf("no ready tasks for project: %s", project.Name)
		}
		if errors.Is(err, storage.ErrTaskNotReady) {
			return fmt.Errorf("task is not ready to claim")
		}
		return err
	}

	if claimOpts.json {
		return printClaimedTaskJSON(c.out, task)
	}

	printTask(c.out, task)
	return nil
}

func parseTaskCreateOptions(args []string) (taskCreateOptions, error) {
	var opts taskCreateOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return taskCreateOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--title":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--title requires a value", Code: 2}
			}
			opts.title = args[i]
		case strings.HasPrefix(arg, "--title="):
			opts.title = strings.TrimPrefix(arg, "--title=")
			if opts.title == "" {
				return taskCreateOptions{}, &UsageError{Message: "--title requires a value", Code: 2}
			}
		case arg == "--description":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--description requires a value", Code: 2}
			}
			opts.description = args[i]
		case strings.HasPrefix(arg, "--description="):
			opts.description = strings.TrimPrefix(arg, "--description=")
		case arg == "--acceptance-criteria":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--acceptance-criteria requires a value", Code: 2}
			}
			opts.acceptanceCriteria = args[i]
		case strings.HasPrefix(arg, "--acceptance-criteria="):
			opts.acceptanceCriteria = strings.TrimPrefix(arg, "--acceptance-criteria=")
		case arg == "--notes":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--notes requires a value", Code: 2}
			}
			opts.notes = args[i]
		case strings.HasPrefix(arg, "--notes="):
			opts.notes = strings.TrimPrefix(arg, "--notes=")
		case arg == "--source":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--source requires a value", Code: 2}
			}
			opts.source = args[i]
		case strings.HasPrefix(arg, "--source="):
			opts.source = strings.TrimPrefix(arg, "--source=")
			if opts.source == "" {
				return taskCreateOptions{}, &UsageError{Message: "--source requires a value", Code: 2}
			}
		case arg == "--external-id":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--external-id requires a value", Code: 2}
			}
			opts.externalID = args[i]
		case strings.HasPrefix(arg, "--external-id="):
			opts.externalID = strings.TrimPrefix(arg, "--external-id=")
			if opts.externalID == "" {
				return taskCreateOptions{}, &UsageError{Message: "--external-id requires a value", Code: 2}
			}
		case arg == "--external-url":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--external-url requires a value", Code: 2}
			}
			opts.externalURL = args[i]
		case strings.HasPrefix(arg, "--external-url="):
			opts.externalURL = strings.TrimPrefix(arg, "--external-url=")
			if opts.externalURL == "" {
				return taskCreateOptions{}, &UsageError{Message: "--external-url requires a value", Code: 2}
			}
		case arg == "--external-revision":
			i++
			if i >= len(args) {
				return taskCreateOptions{}, &UsageError{Message: "--external-revision requires a value", Code: 2}
			}
			opts.externalRevision = args[i]
		case strings.HasPrefix(arg, "--external-revision="):
			opts.externalRevision = strings.TrimPrefix(arg, "--external-revision=")
			if opts.externalRevision == "" {
				return taskCreateOptions{}, &UsageError{Message: "--external-revision requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return taskCreateOptions{}, &UsageError{Message: fmt.Sprintf("unknown task create option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.title = strings.TrimSpace(opts.title)
	opts.source = strings.TrimSpace(opts.source)
	opts.externalID = strings.TrimSpace(opts.externalID)
	opts.externalURL = strings.TrimSpace(opts.externalURL)
	opts.externalRevision = strings.TrimSpace(opts.externalRevision)
	if opts.projectName == "" {
		return taskCreateOptions{}, &UsageError{Message: "task create requires --project", Code: 2}
	}
	if opts.title == "" {
		return taskCreateOptions{}, &UsageError{Message: "task create requires --title", Code: 2}
	}

	return opts, nil
}

func parseTaskSourceOptions(args []string) (taskSourceOptions, error) {
	var opts taskSourceOptions
	if len(args) == 0 {
		return taskSourceOptions{}, &UsageError{Message: "task source requires <task-id>", Code: 2}
	}
	taskID, err := parseTaskID(args[0])
	if err != nil {
		return taskSourceOptions{}, err
	}
	opts.taskID = taskID

	for i := 1; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--source":
			i++
			if i >= len(args) {
				return taskSourceOptions{}, &UsageError{Message: "--source requires a value", Code: 2}
			}
			opts.source = args[i]
		case strings.HasPrefix(arg, "--source="):
			opts.source = strings.TrimPrefix(arg, "--source=")
			if opts.source == "" {
				return taskSourceOptions{}, &UsageError{Message: "--source requires a value", Code: 2}
			}
		case arg == "--external-id":
			i++
			if i >= len(args) {
				return taskSourceOptions{}, &UsageError{Message: "--external-id requires a value", Code: 2}
			}
			opts.externalID = args[i]
		case strings.HasPrefix(arg, "--external-id="):
			opts.externalID = strings.TrimPrefix(arg, "--external-id=")
			if opts.externalID == "" {
				return taskSourceOptions{}, &UsageError{Message: "--external-id requires a value", Code: 2}
			}
		case arg == "--external-url":
			i++
			if i >= len(args) {
				return taskSourceOptions{}, &UsageError{Message: "--external-url requires a value", Code: 2}
			}
			opts.externalURL = args[i]
		case strings.HasPrefix(arg, "--external-url="):
			opts.externalURL = strings.TrimPrefix(arg, "--external-url=")
			if opts.externalURL == "" {
				return taskSourceOptions{}, &UsageError{Message: "--external-url requires a value", Code: 2}
			}
		case arg == "--external-revision":
			i++
			if i >= len(args) {
				return taskSourceOptions{}, &UsageError{Message: "--external-revision requires a value", Code: 2}
			}
			opts.externalRevision = args[i]
		case strings.HasPrefix(arg, "--external-revision="):
			opts.externalRevision = strings.TrimPrefix(arg, "--external-revision=")
			if opts.externalRevision == "" {
				return taskSourceOptions{}, &UsageError{Message: "--external-revision requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return taskSourceOptions{}, &UsageError{Message: fmt.Sprintf("unknown task source option %q", arg), Code: 2}
		}
	}

	opts.source = strings.TrimSpace(opts.source)
	opts.externalID = strings.TrimSpace(opts.externalID)
	opts.externalURL = strings.TrimSpace(opts.externalURL)
	opts.externalRevision = strings.TrimSpace(opts.externalRevision)
	if opts.source == "" {
		return taskSourceOptions{}, &UsageError{Message: "task source requires --source", Code: 2}
	}
	return opts, nil
}

func parseTaskListOptions(args []string) (taskListOptions, error) {
	var opts taskListOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return taskListOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return taskListOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--status":
			i++
			if i >= len(args) {
				return taskListOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
			opts.status = args[i]
		case strings.HasPrefix(arg, "--status="):
			opts.status = strings.TrimPrefix(arg, "--status=")
			if opts.status == "" {
				return taskListOptions{}, &UsageError{Message: "--status requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return taskListOptions{}, &UsageError{Message: fmt.Sprintf("unknown task list option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.status = strings.TrimSpace(opts.status)
	if opts.projectName == "" {
		return taskListOptions{}, &UsageError{Message: "task list requires --project", Code: 2}
	}
	if opts.status != "" && !validTaskStatusOption(opts.status) {
		return taskListOptions{}, &UsageError{Message: fmt.Sprintf("invalid task status %q", opts.status), Code: 2}
	}

	return opts, nil
}

type taskDependencyOptions struct {
	edgeType      string
	blockerTaskID int64
	blockedTaskID int64
	json          bool
}

func parseTaskDependencyOptions(args []string) (taskDependencyOptions, error) {
	var opts taskDependencyOptions
	opts.edgeType = "blocks"

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--type":
			i++
			if i >= len(args) {
				return taskDependencyOptions{}, &UsageError{Message: "--type requires a value", Code: 2}
			}
			opts.edgeType = args[i]
		case strings.HasPrefix(arg, "--type="):
			opts.edgeType = strings.TrimPrefix(arg, "--type=")
			if opts.edgeType == "" {
				return taskDependencyOptions{}, &UsageError{Message: "--type requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return taskDependencyOptions{}, &UsageError{Message: fmt.Sprintf("unknown task dependency option %q", arg), Code: 2}
		default:
			if opts.blockerTaskID == 0 {
				id, err := parseTaskID(arg)
				if err != nil {
					return taskDependencyOptions{}, err
				}
				opts.blockerTaskID = id
				continue
			}
			if opts.blockedTaskID == 0 {
				id, err := parseTaskID(arg)
				if err != nil {
					return taskDependencyOptions{}, err
				}
				opts.blockedTaskID = id
				continue
			}
			return taskDependencyOptions{}, &UsageError{Message: "task dependency accepts exactly blocker and blocked task ids", Code: 2}
		}
	}

	if opts.blockerTaskID == 0 || opts.blockedTaskID == 0 {
		return taskDependencyOptions{}, &UsageError{Message: "task dependency requires blocker and blocked task ids", Code: 2}
	}
	if opts.edgeType != "blocks" {
		return taskDependencyOptions{}, &UsageError{Message: fmt.Sprintf("invalid task dependency edge type %q", opts.edgeType), Code: 2}
	}

	return opts, nil
}

func parseTaskReadyOptions(args []string) (taskReadyOptions, error) {
	var opts taskReadyOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return taskReadyOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return taskReadyOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		default:
			return taskReadyOptions{}, &UsageError{Message: fmt.Sprintf("unknown task ready option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	if opts.projectName == "" {
		return taskReadyOptions{}, &UsageError{Message: "task ready requires --project", Code: 2}
	}

	return opts, nil
}

func parseTaskClaimOptions(args []string) (taskClaimOptions, error) {
	var opts taskClaimOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return taskClaimOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
			opts.projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			opts.projectName = strings.TrimPrefix(arg, "--project=")
			if opts.projectName == "" {
				return taskClaimOptions{}, &UsageError{Message: "--project requires a value", Code: 2}
			}
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return taskClaimOptions{}, &UsageError{Message: fmt.Sprintf("unknown task claim option %q", arg), Code: 2}
		default:
			if opts.taskID != 0 {
				return taskClaimOptions{}, &UsageError{Message: "task claim accepts at most one task id", Code: 2}
			}
			taskID, err := parseTaskID(arg)
			if err != nil {
				return taskClaimOptions{}, err
			}
			opts.taskID = taskID
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	if opts.projectName == "" {
		return taskClaimOptions{}, &UsageError{Message: "task claim requires --project", Code: 2}
	}

	return opts, nil
}

func parseTaskStatusOptions(args []string) (taskStatusOptions, error) {
	var opts taskStatusOptions

	for _, arg := range args {
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return taskStatusOptions{}, &UsageError{Message: fmt.Sprintf("unknown task status option %q", arg), Code: 2}
		default:
			if opts.taskID == 0 {
				taskID, err := parseTaskID(arg)
				if err != nil {
					return taskStatusOptions{}, err
				}
				opts.taskID = taskID
				continue
			}
			if opts.status == "" {
				opts.status = strings.TrimSpace(arg)
				continue
			}
			return taskStatusOptions{}, &UsageError{Message: "task status accepts exactly task id and status", Code: 2}
		}
	}

	if opts.taskID == 0 || opts.status == "" {
		return taskStatusOptions{}, &UsageError{Message: "task status requires a task id and status", Code: 2}
	}
	if !validTaskStatusOption(opts.status) {
		return taskStatusOptions{}, &UsageError{Message: fmt.Sprintf("invalid task status %q", opts.status), Code: 2}
	}

	return opts, nil
}

func parseTaskShowOptions(args []string) (taskShowOptions, error) {
	var opts taskShowOptions

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.json = true
		case strings.HasPrefix(arg, "-"):
			return taskShowOptions{}, &UsageError{Message: fmt.Sprintf("unknown task show option %q", arg), Code: 2}
		default:
			if opts.taskID != 0 {
				return taskShowOptions{}, &UsageError{Message: "task show accepts exactly one task id", Code: 2}
			}
			taskID, err := parseTaskID(arg)
			if err != nil {
				return taskShowOptions{}, err
			}
			opts.taskID = taskID
		}
	}

	if opts.taskID == 0 {
		return taskShowOptions{}, &UsageError{Message: "task show requires a task id", Code: 2}
	}

	return opts, nil
}

func parseTaskDoneOptions(args []string) (taskDoneOptions, error) {
	if len(args) == 0 {
		return taskDoneOptions{}, &UsageError{Message: "task done requires a task id", Code: 2}
	}

	var taskID int64
	var note string
	var jsonOutput bool
	var evidenceRunID int64
	var allowUnvalidated bool
	var overrideReason string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--note":
			i++
			if i >= len(args) {
				return taskDoneOptions{}, &UsageError{Message: "--note requires a value", Code: 2}
			}
			note = args[i]
		case strings.HasPrefix(arg, "--note="):
			note = strings.TrimPrefix(arg, "--note=")
			if note == "" {
				return taskDoneOptions{}, &UsageError{Message: "--note requires a value", Code: 2}
			}
		case arg == "--evidence-run":
			i++
			if i >= len(args) {
				return taskDoneOptions{}, &UsageError{Message: "--evidence-run requires a value", Code: 2}
			}
			id, err := parseRunID(args[i])
			if err != nil {
				return taskDoneOptions{}, err
			}
			evidenceRunID = id
		case strings.HasPrefix(arg, "--evidence-run="):
			id, err := parseRunID(strings.TrimPrefix(arg, "--evidence-run="))
			if err != nil {
				return taskDoneOptions{}, err
			}
			evidenceRunID = id
		case arg == "--allow-unvalidated":
			allowUnvalidated = true
		case arg == "--override-reason":
			i++
			if i >= len(args) {
				return taskDoneOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
			overrideReason = args[i]
		case strings.HasPrefix(arg, "--override-reason="):
			overrideReason = strings.TrimPrefix(arg, "--override-reason=")
			if overrideReason == "" {
				return taskDoneOptions{}, &UsageError{Message: "--override-reason requires a value", Code: 2}
			}
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return taskDoneOptions{}, &UsageError{Message: fmt.Sprintf("unknown task done option %q", arg), Code: 2}
		default:
			if taskID != 0 {
				return taskDoneOptions{}, &UsageError{Message: "task done accepts exactly one task id", Code: 2}
			}
			id, err := parseTaskID(arg)
			if err != nil {
				return taskDoneOptions{}, err
			}
			taskID = id
		}
	}

	if taskID == 0 {
		return taskDoneOptions{}, &UsageError{Message: "task done requires a task id", Code: 2}
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return taskDoneOptions{}, &UsageError{Message: "task done requires --note", Code: 2}
	}
	overrideReason = strings.TrimSpace(overrideReason)
	if allowUnvalidated && overrideReason == "" {
		return taskDoneOptions{}, &UsageError{Message: "task done --allow-unvalidated requires --override-reason", Code: 2}
	}

	return taskDoneOptions{
		taskID:           taskID,
		note:             note,
		evidenceRunID:    evidenceRunID,
		allowUnvalidated: allowUnvalidated,
		overrideReason:   overrideReason,
		json:             jsonOutput,
	}, nil
}

func parseRequiredProjectOption(args []string, command string) (string, error) {
	var projectName string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--project":
			i++
			if i >= len(args) {
				return "", &UsageError{Message: "--project requires a value", Code: 2}
			}
			projectName = args[i]
		case strings.HasPrefix(arg, "--project="):
			projectName = strings.TrimPrefix(arg, "--project=")
			if projectName == "" {
				return "", &UsageError{Message: "--project requires a value", Code: 2}
			}
		default:
			return "", &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		}
	}

	projectName = strings.TrimSpace(projectName)
	if projectName == "" {
		return "", &UsageError{Message: command + " requires --project", Code: 2}
	}

	return projectName, nil
}

func parseTaskCommentOptions(args []string) (taskCommentOptions, error) {
	return parseTaskNoteOptions(args, "task comment", "--body")
}

func parseTaskNoteOptions(args []string, command, flag string) (taskCommentOptions, error) {
	if len(args) == 0 {
		return taskCommentOptions{}, &UsageError{Message: command + " requires a task id", Code: 2}
	}

	var taskID int64
	var body string
	var jsonOutput bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == flag:
			i++
			if i >= len(args) {
				return taskCommentOptions{}, &UsageError{Message: flag + " requires a value", Code: 2}
			}
			body = args[i]
		case strings.HasPrefix(arg, flag+"="):
			body = strings.TrimPrefix(arg, flag+"=")
			if body == "" {
				return taskCommentOptions{}, &UsageError{Message: flag + " requires a value", Code: 2}
			}
		case arg == "--json":
			jsonOutput = true
		case strings.HasPrefix(arg, "-"):
			return taskCommentOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		default:
			if taskID != 0 {
				return taskCommentOptions{}, &UsageError{Message: command + " accepts exactly one task id", Code: 2}
			}
			id, err := parseTaskID(arg)
			if err != nil {
				return taskCommentOptions{}, err
			}
			taskID = id
		}
	}

	if taskID == 0 {
		return taskCommentOptions{}, &UsageError{Message: command + " requires a task id", Code: 2}
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return taskCommentOptions{}, &UsageError{Message: command + " requires " + flag, Code: 2}
	}

	return taskCommentOptions{taskID: taskID, body: body, json: jsonOutput}, nil
}

func getProjectForTask(ctx context.Context, store *storage.Store, name string) (storage.Project, error) {
	project, err := store.GetProject(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return storage.Project{}, fmt.Errorf("project not found: %s", name)
		}
		return storage.Project{}, err
	}
	return project, nil
}

func parseTaskID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, &UsageError{Message: fmt.Sprintf("invalid task id: %s", value), Code: 2}
	}
	return id, nil
}

func printTask(out io.Writer, task storage.Task) {
	fmt.Fprintf(out, "id: %d\n", task.ID)
	fmt.Fprintf(out, "project_id: %d\n", task.ProjectID)
	fmt.Fprintf(out, "status: %s\n", task.Status)
	fmt.Fprintf(out, "title: %s\n", task.Title)
	fmt.Fprintf(out, "description: %s\n", task.Description)
	fmt.Fprintf(out, "acceptance_criteria: %s\n", task.AcceptanceCriteria)
	fmt.Fprintf(out, "notes: %s\n", task.Notes)
	fmt.Fprintf(out, "source: %s\n", task.Source)
	if task.ExternalID != "" {
		fmt.Fprintf(out, "external_id: %s\n", task.ExternalID)
	}
	if task.ExternalURL != "" {
		fmt.Fprintf(out, "external_url: %s\n", task.ExternalURL)
	}
	if task.ExternalRevision != "" {
		fmt.Fprintf(out, "external_revision: %s\n", task.ExternalRevision)
	}
	fmt.Fprintf(out, "created_at: %s\n", task.CreatedAt)
	fmt.Fprintf(out, "updated_at: %s\n", task.UpdatedAt)
}

func printTaskEvents(out io.Writer, events []storage.TaskEvent) {
	if len(events) == 0 {
		fmt.Fprintln(out, "events: none")
		return
	}

	fmt.Fprintln(out, "events:")
	for _, event := range events {
		fmt.Fprintf(out, "- id: %d type: %s", event.ID, event.Type)
		if event.FromStatus != "" && event.ToStatus != "" {
			fmt.Fprintf(out, " from: %s to: %s", event.FromStatus, event.ToStatus)
		} else if event.ToStatus != "" {
			fmt.Fprintf(out, " status: %s", event.ToStatus)
		}
		if event.Body != "" {
			fmt.Fprintf(out, " body: %s", event.Body)
		}
		if event.ActorName != "" {
			fmt.Fprintf(out, " actor: %s/%s", event.ActorKind, event.ActorName)
		}
		fmt.Fprintf(out, " created_at: %s\n", event.CreatedAt)
	}
}

func printTaskEvent(out io.Writer, event storage.TaskEvent) {
	fmt.Fprintf(out, "id: %d\n", event.ID)
	fmt.Fprintf(out, "task_id: %d\n", event.TaskID)
	fmt.Fprintf(out, "type: %s\n", event.Type)
	fmt.Fprintf(out, "body: %s\n", event.Body)
	if event.ActorName != "" {
		fmt.Fprintf(out, "actor_kind: %s\n", event.ActorKind)
		fmt.Fprintf(out, "actor_name: %s\n", event.ActorName)
		fmt.Fprintf(out, "actor_id: %d\n", event.ActorID)
	}
	fmt.Fprintf(out, "created_at: %s\n", event.CreatedAt)
}

func printTaskDependency(out io.Writer, dependency storage.TaskDependency) {
	fmt.Fprintf(out, "id: %d\n", dependency.ID)
	fmt.Fprintf(out, "edge_type: %s\n", dependency.EdgeType)
	fmt.Fprintf(out, "blocker_task_id: %d\n", dependency.BlockerTaskID)
	fmt.Fprintf(out, "blocked_task_id: %d\n", dependency.BlockedTaskID)
	fmt.Fprintf(out, "created_at: %s\n", dependency.CreatedAt)
}

type readyTaskOutput struct {
	ID                 int64  `json:"id"`
	ProjectID          int64  `json:"project_id"`
	Status             string `json:"status"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	AcceptanceCriteria string `json:"acceptance_criteria"`
	Notes              string `json:"notes"`
	Source             string `json:"source"`
	ExternalID         string `json:"external_id"`
	ExternalURL        string `json:"external_url"`
	ExternalRevision   string `json:"external_revision"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type taskEventOutput struct {
	ID         int64        `json:"id"`
	TaskID     int64        `json:"task_id"`
	Type       string       `json:"type"`
	Body       string       `json:"body"`
	FromStatus string       `json:"from_status"`
	ToStatus   string       `json:"to_status"`
	Actor      *actorOutput `json:"actor,omitempty"`
	CreatedAt  string       `json:"created_at"`
}

type actorOutput struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type taskShowOutput struct {
	Task   readyTaskOutput   `json:"task"`
	Events []taskEventOutput `json:"events"`
}

func printReadyTasksJSON(out io.Writer, tasks []storage.Task) error {
	return printTasksJSON(out, tasks)
}

func printTaskJSON(out io.Writer, task storage.Task) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskOutputFromStorage(task))
}

func printTasksJSON(out io.Writer, tasks []storage.Task) error {
	readyTasks := make([]readyTaskOutput, 0, len(tasks))
	for _, task := range tasks {
		readyTasks = append(readyTasks, taskOutputFromStorage(task))
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(readyTasks)
}

func printClaimedTaskJSON(out io.Writer, task storage.Task) error {
	return printTaskJSON(out, task)
}

func printTaskEventJSON(out io.Writer, event storage.TaskEvent) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskEventOutputFromStorage(event))
}

func printTaskDependencyJSON(out io.Writer, dependency storage.TaskDependency) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskDependencyOutputFromStorage(dependency))
}

func printTaskDependencyRemovedJSON(out io.Writer, dependency taskDependencyOptions) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(struct {
		Removed       bool   `json:"removed"`
		EdgeType      string `json:"edge_type"`
		BlockerTaskID int64  `json:"blocker_task_id"`
		BlockedTaskID int64  `json:"blocked_task_id"`
	}{
		Removed:       true,
		EdgeType:      dependency.edgeType,
		BlockerTaskID: dependency.blockerTaskID,
		BlockedTaskID: dependency.blockedTaskID,
	})
}

func printTaskShowJSON(out io.Writer, task storage.Task, events []storage.TaskEvent) error {
	eventOutputs := make([]taskEventOutput, 0, len(events))
	for _, event := range events {
		eventOutputs = append(eventOutputs, taskEventOutputFromStorage(event))
	}

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskShowOutput{
		Task:   taskOutputFromStorage(task),
		Events: eventOutputs,
	})
}

func actorOutputFromSnapshot(id int64, kind, name string) *actorOutput {
	if id <= 0 || kind == "" || name == "" {
		return nil
	}
	return &actorOutput{
		ID:   id,
		Kind: kind,
		Name: name,
	}
}

func taskOutputFromStorage(task storage.Task) readyTaskOutput {
	return readyTaskOutput{
		ID:                 task.ID,
		ProjectID:          task.ProjectID,
		Status:             task.Status,
		Title:              task.Title,
		Description:        task.Description,
		AcceptanceCriteria: task.AcceptanceCriteria,
		Notes:              task.Notes,
		Source:             task.Source,
		ExternalID:         task.ExternalID,
		ExternalURL:        task.ExternalURL,
		ExternalRevision:   task.ExternalRevision,
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
	}
}

func taskEventOutputFromStorage(event storage.TaskEvent) taskEventOutput {
	return taskEventOutput{
		ID:         event.ID,
		TaskID:     event.TaskID,
		Type:       event.Type,
		Body:       event.Body,
		FromStatus: event.FromStatus,
		ToStatus:   event.ToStatus,
		Actor:      actorOutputFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
		CreatedAt:  event.CreatedAt,
	}
}

func validTaskStatusOption(status string) bool {
	switch status {
	case "open", "in_progress", "blocked", "done":
		return true
	default:
		return false
	}
}
