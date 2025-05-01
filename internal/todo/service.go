package todo

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var todos = make(map[string]string)

// must implement type TodoServiceServer interface public methods
type Service struct {
	proto.UnimplementedTodoServiceServer // default
}

func (s *Service) AddTask(ctx context.Context, req *proto.AddTaskRequest) (*proto.AddTaskResponse, error) {
	if req.GetTask() == "" {
		return nil, status.Error(codes.InvalidArgument, "task cannot be empty")
	}
	id := uuid.New().String()
	todos[id] = req.GetTask()

	return &proto.AddTaskResponse{Id: id}, nil
}

func (s *Service) CompleteTask(ctx context.Context, req *proto.CompleteTaskRequest) (*proto.CompleteTaskResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task UUID cannot be empty")
	}
	if _, ok := todos[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("task UUID %s not found", req.GetId()))
	}

	delete(todos, req.GetId())

	return &proto.CompleteTaskResponse{}, nil
}

func (s *Service) ListTasks(ctx context.Context, req *proto.ListTasksRequest) (*proto.ListTasksResponse, error) {
	tasks := make([]*proto.Task, 0, len(todos))

	for id, task := range todos {
		tasks = append(tasks, &proto.Task{Id: id, Task: task})
	}

	return &proto.ListTasksResponse{Tasks: tasks}, nil
}
