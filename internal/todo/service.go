package todo

import (
	"baremetal-ctl/proto"
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// must implement type TodoServiceServer interface public methods
// in gRPC it's common to have a large number of concurrent RPC calls, especially in a Kubernetes platform environment
type service struct {
	proto.UnimplementedTodoServiceServer
	sync.Mutex
	tasks map[string]string
}

// NewService is the only way to construct a service object, keeping it private to this server package
func NewService() *service {
	return &service{tasks: make(map[string]string)}
}

func (s *service) AddTask(ctx context.Context, req *proto.AddTaskRequest) (*proto.AddTaskResponse, error) {
	if req.GetTask() == "" {
		return nil, status.Error(codes.InvalidArgument, "task cannot be empty")
	}

	id := uuid.New().String()
	s.Lock()
	defer s.Unlock()

	s.tasks[id] = req.GetTask()

	return &proto.AddTaskResponse{Id: id}, nil
}

func (s *service) CompleteTask(ctx context.Context, req *proto.CompleteTaskRequest) (*proto.CompleteTaskResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "task UUID cannot be empty")
	}

	s.Lock()
	defer s.Unlock()

	if _, ok := s.tasks[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("task UUID %s not found", req.GetId()))
	}

	delete(s.tasks, req.GetId())

	return &proto.CompleteTaskResponse{}, nil
}

func (s *service) ListTasks(ctx context.Context, req *proto.ListTasksRequest) (*proto.ListTasksResponse, error) {
	s.Lock()
	defer s.Unlock()

	tasks := make([]*proto.Task, 0, len(s.tasks))

	for id, task := range s.tasks {
		tasks = append(tasks, &proto.Task{Id: id, Task: task})
	}

	return &proto.ListTasksResponse{Tasks: tasks}, nil
}
