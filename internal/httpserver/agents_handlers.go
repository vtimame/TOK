package httpserver

import (
	"strings"

	"github.com/go-fuego/fuego"
	"s26.sh/tok/internal/storage"
)

func (a *api) health(_ fuego.ContextNoBody) (HealthOutput, error) {
	return HealthOutput{Status: "ok", Version: a.version}, nil
}

func (a *api) listAgents(ctx fuego.ContextNoBody) (AgentListResponse, error) {
	agents, err := a.store.ListAgentActivity(ctx.Context())
	if err != nil {
		return AgentListResponse{}, err
	}
	projects, err := a.store.ListAgentProjectActivity(ctx.Context())
	if err != nil {
		return AgentListResponse{}, err
	}
	return agentListFromStorage(agents, projects), nil
}

func (a *api) createAgent(ctx fuego.ContextWithBody[CreateAgentInput]) (CreateAgentResponse, error) {
	body, err := ctx.Body()
	if err != nil {
		return CreateAgentResponse{}, err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		return CreateAgentResponse{}, badRequest("agent name is required")
	}

	created, err := a.store.CreateAgent(ctx.Context(), name)
	if err != nil {
		return CreateAgentResponse{}, mapAgentWriteError(err)
	}
	agentOut, err := a.agentOutput(ctx.Context(), created.Agent)
	if err != nil {
		return CreateAgentResponse{}, err
	}
	return CreateAgentResponse{Agent: agentOut, Token: created.Token}, nil
}

func (a *api) showAgent(ctx fuego.ContextNoBody) (AgentResponse, error) {
	agent, err := a.agentByID(ctx.Context(), ctx.PathParam("id"))
	if err != nil {
		return AgentResponse{}, err
	}
	agentOut, err := a.agentOutput(ctx.Context(), agent)
	if err != nil {
		return AgentResponse{}, err
	}
	return AgentResponse{Agent: agentOut}, nil
}

func (a *api) updateAgent(ctx fuego.ContextWithBody[UpdateAgentInput]) (AgentResponse, error) {
	current, err := a.agentByID(ctx.Context(), ctx.PathParam("id"))
	if err != nil {
		return AgentResponse{}, err
	}
	body, err := ctx.Body()
	if err != nil {
		return AgentResponse{}, err
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = current.Name
	}

	agent, err := a.store.UpdateAgent(ctx.Context(), current.ID, storage.UpdateAgentInput{Name: name})
	if err != nil {
		return AgentResponse{}, mapAgentWriteError(err)
	}
	agentOut, err := a.agentOutput(ctx.Context(), agent)
	if err != nil {
		return AgentResponse{}, err
	}
	return AgentResponse{Agent: agentOut}, nil
}

func (a *api) deleteAgent(ctx fuego.ContextNoBody) (AgentResponse, error) {
	agent, err := a.agentByID(ctx.Context(), ctx.PathParam("id"))
	if err != nil {
		return AgentResponse{}, err
	}
	agentOut, err := a.agentOutput(ctx.Context(), agent)
	if err != nil {
		return AgentResponse{}, err
	}
	if err := a.store.DeleteAgent(ctx.Context(), agent.ID); err != nil {
		return AgentResponse{}, mapAgentWriteError(err)
	}
	return AgentResponse{Agent: agentOut}, nil
}
