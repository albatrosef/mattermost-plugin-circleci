package plugin

import (
	"fmt"
	"net/http"

	"github.com/mattermost/mattermost-server/v5/model"

	"github.com/mattermost/mattermost-plugin-circleci/server/circle"
)

func (p *Plugin) httpHandleApprove(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-Id")
	circleciToken, err := p.Store.GetTokenForUser(userID, p.getConfiguration().EncryptionKey)
	if err != nil {
		p.API.LogError("Error when getting token", err)
	}

	if circleciToken == "" {
		http.NotFound(w, r)
	}

	requestData := model.PostActionIntegrationRequestFromJson(r.Body)
	if requestData == nil {
		p.API.LogError("Empty request data")
		return
	}

	username := ""
	if user, appErr := p.API.GetUser(userID); appErr != nil {
		p.API.LogError("Unable to get user", "userID", userID)
	} else {
		username = user.Username
	}

	originalPost, appErr := p.API.GetPost(requestData.PostId)
	if appErr != nil {
		p.API.LogError("Unable to get post", "postID", requestData.PostId)
	} else {
		newAttachments := []*model.SlackAttachment{}
		for _, attach := range originalPost.Attachments() {
			filteredAttach := attach
			filteredAttach.Actions = nil
			for _, action := range attach.Actions {
				if action.Id != "approvecirclecijob" {
					filteredAttach.Actions = append(filteredAttach.Actions, action)
				}
			}

			filteredAttach.Color = "#50F100" // green
			filteredAttach.Title = fmt.Sprintf("This CircleCI workflow have been approved by %s", username)
			newAttachments = append(newAttachments, filteredAttach)
		}
		originalPost.DelProp("attachments")
		originalPost.AddProp("attachments", newAttachments)

		if _, appErr := p.API.UpdatePost(originalPost); appErr != nil {
			p.API.LogError("Unable to update post", "postID", originalPost.Id)
		}
	}

	responsePost := &model.Post{
		ChannelId: requestData.ChannelId,
		RootId:    requestData.PostId,
		UserId:    p.botUserID,
	}

	workFlowID := fmt.Sprintf("%v", requestData.Context["WorkflowID"])
	jobs, err := circle.GetWorkflowJobs(circleciToken, workFlowID)

	if err != nil {
		p.API.LogError("Error occurred while getting workflow jobs", err)
		responsePost.Message = fmt.Sprintf("Cannot approve the Job from mattermost. Please approve [here](https://circleci.com/workflow-run/%s)", workFlowID)
		if _, appErr := p.API.CreatePost(responsePost); appErr != nil {
			p.API.LogError("Error when creating post", "appError", appErr)
		}
		return
	}

	var approvalRequestID string
	for _, job := range *jobs {
		if job.ApprovalRequestId != "" {
			approvalRequestID = fmt.Sprintf("%v", job.ApprovalRequestId)
		}
	}

	responsePost.Message = fmt.Sprintf("Job successfully approved by %s :+1:", username)
	if _, err = circle.ApproveJob(circleciToken, approvalRequestID, workFlowID); err != nil {
		p.API.LogError("Error occurred while approving", err)
		responsePost.Message = fmt.Sprintf("Cannot approve the Job from mattermost. Please approve [here](https://circleci.com/workflow-run/%s)", workFlowID)
	}

	if _, appErr := p.API.CreatePost(responsePost); appErr != nil {
		p.API.LogError("Error when creating post", "appError", appErr)
	}
}
