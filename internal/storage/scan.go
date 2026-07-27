package storage

type scanner interface {
	Scan(dest ...any) error
}

func scanProject(row scanner) (Project, error) {
	var project Project
	if err := row.Scan(&project.ID, &project.Name, &project.DisplayName, &project.Path, &project.CreatedAt, &project.UpdatedAt); err != nil {
		return Project{}, err
	}
	return project, nil
}

func scanTask(row scanner) (Task, error) {
	var task Task
	if err := row.Scan(&task.ID, &task.ProjectID, &task.Status, &task.Title, &task.Description, &task.AcceptanceCriteria, &task.Notes, &task.CreatedAt, &task.UpdatedAt); err != nil {
		return Task{}, err
	}
	return task, nil
}

func scanTaskDependency(row scanner) (TaskDependency, error) {
	var dependency TaskDependency
	if err := row.Scan(&dependency.ID, &dependency.EdgeType, &dependency.BlockerTaskID, &dependency.BlockedTaskID, &dependency.CreatedAt); err != nil {
		return TaskDependency{}, err
	}
	return dependency, nil
}

func scanRun(row scanner) (Run, error) {
	var run Run
	if err := row.Scan(
		&run.ID,
		&run.TaskID,
		&run.Status,
		&run.HandoffContractVersion,
		&run.RetrievalLimit,
		&run.StartedAt,
		&run.FinishedAt,
		&run.BaseBranch,
		&run.BaseHead,
		&run.ResultSummary,
		&run.LeaseOwner,
		&run.HeartbeatAt,
		&run.ExpiresAt,
		&run.ActorID,
		&run.ActorKind,
		&run.ActorName,
		&run.FinishedActorID,
		&run.FinishedActorKind,
		&run.FinishedActorName,
	); err != nil {
		return Run{}, err
	}
	return run, nil
}

func scanRunArtifact(row scanner) (RunArtifact, error) {
	var artifact RunArtifact
	var truncated int
	if err := row.Scan(
		&artifact.ID,
		&artifact.RunID,
		&artifact.Kind,
		&artifact.Path,
		&artifact.ContentHash,
		&artifact.SizeBytes,
		&truncated,
		&artifact.Metadata,
		&artifact.ActorID,
		&artifact.ActorKind,
		&artifact.ActorName,
		&artifact.CreatedAt,
	); err != nil {
		return RunArtifact{}, err
	}
	artifact.Truncated = truncated != 0
	return artifact, nil
}

func scanContextSource(row scanner) (ContextSource, error) {
	var source ContextSource
	if err := row.Scan(&source.ID, &source.ProjectID, &source.Kind, &source.URI, &source.Metadata, &source.CreatedAt, &source.UpdatedAt); err != nil {
		return ContextSource{}, err
	}
	return source, nil
}

func scanProjectInstruction(row scanner) (ProjectInstruction, error) {
	var instruction ProjectInstruction
	var enabled int
	if err := row.Scan(
		&instruction.ID,
		&instruction.ProjectID,
		&instruction.Scope,
		&instruction.Title,
		&instruction.Body,
		&instruction.Priority,
		&enabled,
		&instruction.Source,
		&instruction.CreatedAt,
		&instruction.UpdatedAt,
	); err != nil {
		return ProjectInstruction{}, err
	}
	instruction.Enabled = enabled == 1
	return instruction, nil
}

func scanIndexMetadata(row scanner) (IndexMetadata, error) {
	var metadata IndexMetadata
	if err := row.Scan(&metadata.ID, &metadata.ProjectID, &metadata.SourceID, &metadata.Key, &metadata.Value, &metadata.UpdatedAt); err != nil {
		return IndexMetadata{}, err
	}
	return metadata, nil
}
