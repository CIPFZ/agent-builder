package server

import (
	"encoding/json"
	"net/http"

	"github.com/CIPFZ/agent-builder/internal/apitypes"
)

// handlePostWorkspaceConfigSet sets a configuration field.
//
//	@Summary		Set a config field
//	@Tags			config
//	@Accept			json
//	@Param			id		path	string					true	"Workspace ID"
//	@Param			request	body	apitypes.ConfigSetRequest	true	"Config set request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/set [post]
func (c *controllerV1) handlePostWorkspaceConfigSet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ConfigSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	if err := c.workbench.SetConfigField(id, req.Scope, req.Key, req.Value); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceConfigRemove removes a configuration field.
//
//	@Summary		Remove a config field
//	@Tags			config
//	@Accept			json
//	@Param			id		path	string						true	"Workspace ID"
//	@Param			request	body	apitypes.ConfigRemoveRequest	true	"Config remove request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/remove [post]
func (c *controllerV1) handlePostWorkspaceConfigRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ConfigRemoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	if err := c.workbench.RemoveConfigField(id, req.Scope, req.Key); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceConfigModel updates the preferred model.
//
//	@Summary		Set the preferred model
//	@Tags			config
//	@Accept			json
//	@Param			id		path	string						true	"Workspace ID"
//	@Param			request	body	apitypes.ConfigModelRequest	true	"Config model request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/model [post]
func (c *controllerV1) handlePostWorkspaceConfigModel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ConfigModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	if err := c.workbench.UpdatePreferredModel(id, req.Scope, req.ModelType, req.Model); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceConfigProviderKey sets a provider API key.
//
//	@Summary		Set provider API key
//	@Tags			config
//	@Accept			json
//	@Param			id		path	string							true	"Workspace ID"
//	@Param			request	body	apitypes.ConfigProviderKeyRequest	true	"Config provider key request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/provider-key [post]
func (c *controllerV1) handlePostWorkspaceConfigProviderKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ConfigProviderKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	apiKey, err := req.DecodeAPIKey()
	if err != nil {
		c.server.logError(r, "Failed to decode api key", "error", err, "kind", req.Kind)
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := c.workbench.SetProviderAPIKey(id, req.Scope, req.ProviderID, apiKey); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceConfigImportCopilot imports Copilot credentials.
//
//	@Summary		Import Copilot credentials
//	@Tags			config
//	@Produce		json
//	@Param			id	path		string						true	"Workspace ID"
//	@Success		200	{object}	apitypes.ImportCopilotResponse
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/import-copilot [post]
func (c *controllerV1) handlePostWorkspaceConfigImportCopilot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	token, ok, err := c.workbench.ImportCopilot(id)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, apitypes.ImportCopilotResponse{Token: token, Success: ok})
}

// handlePostWorkspaceConfigRefreshOAuth refreshes an OAuth token for a provider.
//
//	@Summary		Refresh OAuth token
//	@Tags			config
//	@Accept			json
//	@Param			id		path	string							true	"Workspace ID"
//	@Param			request	body	apitypes.ConfigRefreshOAuthRequest	true	"Refresh OAuth request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/config/refresh-oauth [post]
func (c *controllerV1) handlePostWorkspaceConfigRefreshOAuth(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ConfigRefreshOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	if err := c.workbench.RefreshOAuthToken(r.Context(), id, req.Scope, req.ProviderID); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleGetWorkspaceProjectNeedsInit reports whether a project needs initialization.
//
//	@Summary		Check if project needs initialization
//	@Tags			project
//	@Produce		json
//	@Param			id	path		string							true	"Workspace ID"
//	@Success		200	{object}	apitypes.ProjectNeedsInitResponse
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/project/needs-init [get]
func (c *controllerV1) handleGetWorkspaceProjectNeedsInit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	needs, err := c.workbench.ProjectNeedsInitialization(id)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, apitypes.ProjectNeedsInitResponse{NeedsInit: needs})
}

// handlePostWorkspaceProjectInit marks the project as initialized.
//
//	@Summary		Mark project as initialized
//	@Tags			project
//	@Param			id	path	string	true	"Workspace ID"
//	@Success		200
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/project/init [post]
func (c *controllerV1) handlePostWorkspaceProjectInit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := c.workbench.MarkProjectInitialized(id); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleGetWorkspaceProjectInitPrompt returns the project initialization prompt.
//
//	@Summary		Get project initialization prompt
//	@Tags			project
//	@Produce		json
//	@Param			id	path		string							true	"Workspace ID"
//	@Success		200	{object}	apitypes.ProjectInitPromptResponse
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/project/init-prompt [get]
func (c *controllerV1) handleGetWorkspaceProjectInitPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	prompt, err := c.workbench.InitializePrompt(id)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, apitypes.ProjectInitPromptResponse{Prompt: prompt})
}

// handleGetWorkspaceSkills returns the effective visible skills for a workspace.
//
//	@Summary		List visible skills
//	@Tags			skills
//	@Produce		json
//	@Param			id	path		string				true	"Workspace ID"
//	@Success		200	{array}		apitypes.SkillInfo
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/skills [get]
func (c *controllerV1) handleGetWorkspaceSkills(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	skills, err := c.workbench.ListSkills(id)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, skills)
}

// handlePostWorkspaceSkillRead reads a skill's content by ID.
//
//	@Summary		Read skill content
//	@Tags			skills
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Workspace ID"
//	@Param			request	body		apitypes.ReadSkillRequest		true	"Read skill request"
//	@Success		200		{object}	apitypes.ReadSkillResponse
//	@Failure		400		{object}	apitypes.Error
//	@Failure		404		{object}	apitypes.Error
//	@Failure		500		{object}	apitypes.Error
//	@Router			/workspaces/{id}/skills/read [post]
func (c *controllerV1) handlePostWorkspaceSkillRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.ReadSkillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	content, result, err := c.workbench.ReadSkill(r.Context(), id, req.SkillID)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, apitypes.ReadSkillResponse{Content: content, Result: result})
}

// handlePostWorkspaceMCPEnableDocker enables the Docker MCP server.
//
//	@Summary		Enable Docker MCP
//	@Tags			mcp
//	@Param			id	path	string	true	"Workspace ID"
//	@Success		200
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/docker/enable [post]
func (c *controllerV1) handlePostWorkspaceMCPEnableDocker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := c.workbench.EnableDockerMCP(r.Context(), id); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceMCPDisableDocker disables the Docker MCP server.
//
//	@Summary		Disable Docker MCP
//	@Tags			mcp
//	@Param			id	path	string	true	"Workspace ID"
//	@Success		200
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/docker/disable [post]
func (c *controllerV1) handlePostWorkspaceMCPDisableDocker(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := c.workbench.DisableDockerMCP(id); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceMCPRefreshTools refreshes tools for a named MCP server.
//
//	@Summary		Refresh MCP tools
//	@Tags			mcp
//	@Accept			json
//	@Param			id		path	string					true	"Workspace ID"
//	@Param			request	body	apitypes.MCPNameRequest	true	"MCP name request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/refresh-tools [post]
func (c *controllerV1) handlePostWorkspaceMCPRefreshTools(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.MCPNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	if err := c.workbench.RefreshMCPTools(r.Context(), id, req.Name); err != nil {
		c.handleError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceMCPReadResource reads a resource from an MCP server.
//
//	@Summary		Read MCP resource
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Workspace ID"
//	@Param			request	body		apitypes.MCPReadResourceRequest	true	"MCP read resource request"
//	@Success		200		{object}	object
//	@Failure		400		{object}	apitypes.Error
//	@Failure		404		{object}	apitypes.Error
//	@Failure		500		{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/read-resource [post]
func (c *controllerV1) handlePostWorkspaceMCPReadResource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.MCPReadResourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	contents, err := c.workbench.ReadMCPResource(r.Context(), id, req.Name, req.URI)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, contents)
}

// handlePostWorkspaceMCPGetPrompt retrieves a prompt from an MCP server.
//
//	@Summary		Get MCP prompt
//	@Tags			mcp
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string						true	"Workspace ID"
//	@Param			request	body		apitypes.MCPGetPromptRequest	true	"MCP get prompt request"
//	@Success		200		{object}	apitypes.MCPGetPromptResponse
//	@Failure		400		{object}	apitypes.Error
//	@Failure		404		{object}	apitypes.Error
//	@Failure		500		{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/get-prompt [post]
func (c *controllerV1) handlePostWorkspaceMCPGetPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.MCPGetPromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	prompt, err := c.workbench.GetMCPPrompt(id, req.ClientID, req.PromptID, req.Args)
	if err != nil {
		c.handleError(w, r, err)
		return
	}
	jsonEncode(w, apitypes.MCPGetPromptResponse{Prompt: prompt})
}

// handleGetWorkspaceMCPStates returns the state of all MCP clients.
//
//	@Summary		Get MCP client states
//	@Tags			mcp
//	@Produce		json
//	@Param			id	path		string						true	"Workspace ID"
//	@Success		200	{object}	map[string]apitypes.MCPClientInfo
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/states [get]
func (c *controllerV1) handleGetWorkspaceMCPStates(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	states := c.workbench.MCPGetStates(id)
	result := make(map[string]apitypes.MCPClientInfo, len(states))
	for k, v := range states {
		result[k] = apitypes.MCPClientInfo{
			Name:          v.Name,
			State:         apitypes.MCPState(v.State),
			Error:         v.Error,
			ToolCount:     v.Counts.Tools,
			PromptCount:   v.Counts.Prompts,
			ResourceCount: v.Counts.Resources,
			ConnectedAt:   v.ConnectedAt,
		}
	}
	jsonEncode(w, result)
}

// handlePostWorkspaceMCPRefreshPrompts refreshes prompts for a named MCP server.
//
//	@Summary		Refresh MCP prompts
//	@Tags			mcp
//	@Accept			json
//	@Param			id		path	string					true	"Workspace ID"
//	@Param			request	body	apitypes.MCPNameRequest	true	"MCP name request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/refresh-prompts [post]
func (c *controllerV1) handlePostWorkspaceMCPRefreshPrompts(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.MCPNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	c.workbench.MCPRefreshPrompts(r.Context(), id, req.Name)
	w.WriteHeader(http.StatusOK)
}

// handlePostWorkspaceMCPRefreshResources refreshes resources for a named MCP server.
//
//	@Summary		Refresh MCP resources
//	@Tags			mcp
//	@Accept			json
//	@Param			id		path	string					true	"Workspace ID"
//	@Param			request	body	apitypes.MCPNameRequest	true	"MCP name request"
//	@Success		200
//	@Failure		400	{object}	apitypes.Error
//	@Failure		404	{object}	apitypes.Error
//	@Failure		500	{object}	apitypes.Error
//	@Router			/workspaces/{id}/mcp/refresh-resources [post]
func (c *controllerV1) handlePostWorkspaceMCPRefreshResources(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req apitypes.MCPNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.server.logError(r, "Failed to decode request", "error", err)
		jsonError(w, http.StatusBadRequest, "failed to decode request")
		return
	}

	c.workbench.MCPRefreshResources(r.Context(), id, req.Name)
	w.WriteHeader(http.StatusOK)
}
