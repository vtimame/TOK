package httpserver

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-fuego/fuego"

	"s26.sh/tok/internal/retrieval"
	tokservice "s26.sh/tok/internal/service"
	"s26.sh/tok/internal/storage"
)

const (
	defaultAddr = "127.0.0.1:7654"
)

type Config struct {
	Addr    string
	Store   *storage.Store
	Version string
	WebFS   fs.FS
}

type Server struct {
	api    *api
	server *fuego.Server
}

type api struct {
	store     *storage.Store
	tasks     *tokservice.TaskService
	retrieval *retrieval.Service
	version   string
}

func New(cfg Config) (*Server, error) {
	if cfg.Store == nil {
		return nil, errors.New("http server store is required")
	}
	addr := strings.TrimSpace(cfg.Addr)
	if addr == "" {
		addr = defaultAddr
	}
	version := strings.TrimSpace(cfg.Version)
	if version == "" {
		version = "dev"
	}
	webFS, err := resolveWebFS(cfg.WebFS)
	if err != nil {
		return nil, err
	}

	s := fuego.NewServer(
		fuego.WithAddr(addr),
		fuego.WithoutLogger(),
		fuego.WithEngineOptions(fuego.WithOpenAPIConfig(fuego.OpenAPIConfig{
			DisableLocalSave: true,
			PrettyFormatJSON: true,
		})),
	)
	a := &api{
		store:     cfg.Store,
		tasks:     tokservice.NewTaskService(cfg.Store),
		retrieval: retrieval.NewService(cfg.Store),
		version:   version,
	}
	registerRoutes(s, a)
	registerWebRoutes(s, webFS)

	return &Server{api: a, server: s}, nil
}

func (s *Server) Run(ctx context.Context) error {
	if s == nil || s.server == nil {
		return errors.New("http server is nil")
	}
	return s.server.RunContext(ctx)
}

func (s *Server) Handler() http.Handler {
	if s == nil || s.server == nil {
		return http.NewServeMux()
	}
	s.server.OutputOpenAPISpec()
	s.server.RegisterOpenAPIRoutes(s.server)
	return s.server.Mux
}
