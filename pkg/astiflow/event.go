package astiflow

import "github.com/asticode/go-astikit"

const (
	EventNameFlowClosed        astikit.EventName = "astiflow.flow.closed"
	EventNameFlowDone          astikit.EventName = "astiflow.flow.done"
	EventNameFlowRunning       astikit.EventName = "astiflow.flow.running"
	EventNameFlowStarting      astikit.EventName = "astiflow.flow.starting"
	EventNameFlowStopping      astikit.EventName = "astiflow.flow.stopping"
	EventNameGroupClosed       astikit.EventName = "astiflow.group.closed"
	EventNameGroupCreated      astikit.EventName = "astiflow.group.created"
	EventNameGroupDone         astikit.EventName = "astiflow.group.done"
	EventNameGroupRunning      astikit.EventName = "astiflow.group.running"
	EventNameGroupStarting     astikit.EventName = "astiflow.group.starting"
	EventNameGroupStopping     astikit.EventName = "astiflow.group.stopping"
	EventNameNodeChildAdded    astikit.EventName = "astiflow.node.child.added"
	EventNameNodeChildRemoved  astikit.EventName = "astiflow.node.child.removed"
	EventNameNodeClosed        astikit.EventName = "astiflow.node.closed"
	EventNameNodeCreated       astikit.EventName = "astiflow.node.created"
	EventNameNodeDone          astikit.EventName = "astiflow.node.done"
	EventNameNodeParentAdded   astikit.EventName = "astiflow.node.parent.added"
	EventNameNodeParentRemoved astikit.EventName = "astiflow.node.parent.removed"
	EventNameNodeRunning       astikit.EventName = "astiflow.node.running"
	EventNameNodeStarting      astikit.EventName = "astiflow.node.starting"
	EventNameNodeStopping      astikit.EventName = "astiflow.node.stopping"
	eventNameTaskClosed        astikit.EventName = "astiflow.task.closed"
	eventNameTaskDone          astikit.EventName = "astiflow.task.done"
	eventNameTaskRunning       astikit.EventName = "astiflow.task.running"
	eventNameTaskStarting      astikit.EventName = "astiflow.task.starting"
	eventNameTaskStopping      astikit.EventName = "astiflow.task.stopping"
)
