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
}

type taskListOptions struct {
	projectName string
	status      string
	json        bool
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
		Actor:              actor,
	})
	if err != nil {
		return err
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

	fmt.Fprintln(c.out, "id\tstatus\ttitle")
	for _, task := range tasks {
		fmt.Fprintf(c.out, "%d\t%s\t%s\n", task.ID, task.Status, task.Title)
	}
	return nil
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
	if len(args) != 2 {
		return &UsageError{Message: "task status requires a task id and status", Code: 2}
	}

	taskID, err := parseTaskID(args[0])
	if err != nil {
		return err
	}

	actor, err := currentLocalHumanActor(ctx, store)
	if err != nil {
		return err
	}

	task, err := store.UpdateTaskStatusByActor(ctx, taskID, args[1], actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", taskID)
		}
		return err
	}

	printTask(c.out, task)
	return nil
}

type taskDoneOptions struct {
	taskID int64
	note   string
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

	task, err := store.CompleteTaskByActor(ctx, doneOpts.taskID, doneOpts.note, actor)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("task not found: %d", doneOpts.taskID)
		}
		if errors.Is(err, storage.ErrInvalidTaskTransition) {
			return fmt.Errorf("task must be in_progress to complete")
		}
		return err
	}

	printTask(c.out, task)
	return nil
}

type taskCommentOptions struct {
	taskID int64
	body   string
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
		printTaskDependency(c.out, dependency)
		return nil
	case "remove":
		if err := store.RemoveTaskDependency(ctx, dependencyOpts.edgeType, dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("task dependency not found: %d blocks %d", dependencyOpts.blockerTaskID, dependencyOpts.blockedTaskID)
			}
			return err
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

	fmt.Fprintln(c.out, "id\tstatus\ttitle")
	for _, task := range tasks {
		fmt.Fprintf(c.out, "%d\t%s\t%s\n", task.ID, task.Status, task.Title)
	}
	return nil
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
		default:
			return taskCreateOptions{}, &UsageError{Message: fmt.Sprintf("unknown task create option %q", arg), Code: 2}
		}
	}

	opts.projectName = strings.TrimSpace(opts.projectName)
	opts.title = strings.TrimSpace(opts.title)
	if opts.projectName == "" {
		return taskCreateOptions{}, &UsageError{Message: "task create requires --project", Code: 2}
	}
	if opts.title == "" {
		return taskCreateOptions{}, &UsageError{Message: "task create requires --title", Code: 2}
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

	taskID, err := parseTaskID(args[0])
	if err != nil {
		return taskDoneOptions{}, err
	}

	var note string
	for i := 1; i < len(args); i++ {
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
		default:
			return taskDoneOptions{}, &UsageError{Message: fmt.Sprintf("unknown task done option %q", arg), Code: 2}
		}
	}

	note = strings.TrimSpace(note)
	if note == "" {
		return taskDoneOptions{}, &UsageError{Message: "task done requires --note", Code: 2}
	}

	return taskDoneOptions{taskID: taskID, note: note}, nil
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

	taskID, err := parseTaskID(args[0])
	if err != nil {
		return taskCommentOptions{}, err
	}

	var body string
	for i := 1; i < len(args); i++ {
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
		default:
			return taskCommentOptions{}, &UsageError{Message: fmt.Sprintf("unknown %s option %q", command, arg), Code: 2}
		}
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return taskCommentOptions{}, &UsageError{Message: command + " requires " + flag, Code: 2}
	}

	return taskCommentOptions{taskID: taskID, body: body}, nil
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
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(taskOutputFromStorage(task))
}

func printTaskShowJSON(out io.Writer, task storage.Task, events []storage.TaskEvent) error {
	eventOutputs := make([]taskEventOutput, 0, len(events))
	for _, event := range events {
		eventOutputs = append(eventOutputs, taskEventOutput{
			ID:         event.ID,
			TaskID:     event.TaskID,
			Type:       event.Type,
			Body:       event.Body,
			FromStatus: event.FromStatus,
			ToStatus:   event.ToStatus,
			Actor:      actorOutputFromSnapshot(event.ActorID, event.ActorKind, event.ActorName),
			CreatedAt:  event.CreatedAt,
		})
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
		CreatedAt:          task.CreatedAt,
		UpdatedAt:          task.UpdatedAt,
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
