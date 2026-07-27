package httpserver

import (
	"github.com/go-fuego/fuego"

	"s26.sh/tok/internal/retrieval"
)

func (a *api) indexStatus(ctx fuego.ContextNoBody) (IndexResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexResponse{}, err
	}
	status, err := a.retrieval.IndexStatus(ctx.Context(), project)
	if err != nil {
		return IndexResponse{}, err
	}
	return indexFromStatus(status), nil
}

func (a *api) indexUpdate(ctx fuego.ContextNoBody) (IndexResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexResponse{}, err
	}
	summary, err := a.retrieval.IndexProject(ctx.Context(), project)
	if err != nil {
		return IndexResponse{}, err
	}
	return indexFromSummary(summary), nil
}

func (a *api) indexStatusAll(ctx fuego.ContextNoBody) (IndexListResponse, error) {
	projects, err := a.store.ListProjects(ctx.Context())
	if err != nil {
		return IndexListResponse{}, err
	}
	statuses := make([]retrieval.IndexStatus, 0, len(projects))
	for _, project := range projects {
		status, err := a.retrieval.IndexStatus(ctx.Context(), project)
		if err != nil {
			status = retrieval.IndexStatus{
				ProjectName:    project.Name,
				State:          retrieval.StateFailed,
				SkippedReasons: map[string]int{},
				LastError:      err.Error(),
			}
		}
		statuses = append(statuses, status)
	}
	return indexListFromStatuses(statuses), nil
}

func (a *api) indexUpdateAll(ctx fuego.ContextNoBody) (IndexListResponse, error) {
	projects, err := a.store.ListProjects(ctx.Context())
	if err != nil {
		return IndexListResponse{}, err
	}
	summaries := make([]retrieval.IndexSummary, 0, len(projects))
	for _, project := range projects {
		summary, err := a.retrieval.IndexProject(ctx.Context(), project)
		if err != nil {
			summary = retrieval.IndexSummary{
				ProjectName:    project.Name,
				State:          retrieval.StateFailed,
				SkippedReasons: map[string]int{},
				LastError:      err.Error(),
			}
		}
		summaries = append(summaries, summary)
	}
	return indexListFromSummaries(summaries), nil
}

func (a *api) indexIgnorePolicy(ctx fuego.ContextNoBody) (IndexIgnorePolicyResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	policy, err := a.retrieval.IgnorePolicy(ctx.Context(), project)
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	return indexIgnorePolicyFromRetrieval(policy), nil
}

func (a *api) indexIgnoreRefresh(ctx fuego.ContextNoBody) (IndexIgnorePolicyResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	policy, err := a.retrieval.RefreshIgnorePolicy(ctx.Context(), project)
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	return indexIgnorePolicyFromRetrieval(policy), nil
}

func (a *api) indexIgnoreAdd(ctx fuego.ContextWithBody[IndexIgnorePatternInput]) (IndexIgnorePolicyResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	policy, err := a.retrieval.AddIgnorePattern(ctx.Context(), project, body.Pattern)
	if err != nil {
		return IndexIgnorePolicyResponse{}, badRequest(err.Error())
	}
	return indexIgnorePolicyFromRetrieval(policy), nil
}

func (a *api) indexIgnoreRemove(ctx fuego.ContextWithBody[IndexIgnorePatternInput]) (IndexIgnorePolicyResponse, error) {
	project, err := a.projectByName(ctx.Context(), ctx.PathParam("project"))
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return IndexIgnorePolicyResponse{}, err
	}
	policy, err := a.retrieval.RemoveIgnorePattern(ctx.Context(), project, body.Pattern)
	if err != nil {
		return IndexIgnorePolicyResponse{}, badRequest(err.Error())
	}
	return indexIgnorePolicyFromRetrieval(policy), nil
}
