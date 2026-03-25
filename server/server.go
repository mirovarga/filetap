package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/mirovarga/filetap/db"
	"github.com/mirovarga/filetap/source"

	"github.com/charmbracelet/log"
	"github.com/samber/lo"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/render"
)

type dataResponse[T any] struct {
	Meta meta `json:"meta"`
	Data T    `json:"data"`
}

func (d *dataResponse[T]) Render(_ http.ResponseWriter, _ *http.Request) error {
	return nil
}

type meta struct {
	Skip  int `json:"skip"`
	Limit int `json:"limit"`
	Total int `json:"total"`
}

type errResponse struct {
	HTTPStatusCode int         `json:"-"`
	Error          errorDetail `json:"error"`
}

func (e *errResponse) Render(_ http.ResponseWriter, req *http.Request) error {
	render.Status(req, e.HTTPStatusCode)
	return nil
}

type errorDetail struct {
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

type fileLinks struct {
	Self string `json:"self"`
	Raw  string `json:"raw"`
}

type fileWithLinks struct {
	*source.FileInfo
	Links fileLinks `json:"links"`
}

func newFileLinks(hash string) fileLinks {
	return fileLinks{
		Self: "/api/files/" + hash,
		Raw:  "/api/files/" + hash + "/raw",
	}
}

const (
	gzipCompressionLevel  = 5
	maxConcurrentRequests = 100
	requestTimeout        = 60 * time.Second
	shutdownTimeout       = 10 * time.Second
	readHeaderTimeout     = 10 * time.Second
	corsMaxAge            = 300
)

// Server serves the REST API for querying and retrieving indexed files.
type Server struct {
	source      source.Source
	db          *db.DB
	router      chi.Router
	addr        string
	corsOrigins []string
	logger      *log.Logger
}

// New creates a Server with the given configuration and wires up routes.
func New(port int, corsOrigins []string, database *db.DB, source source.Source, logger *log.Logger) *Server {
	server := &Server{
		source:      source,
		db:          database,
		addr:        fmt.Sprintf(":%d", port),
		corsOrigins: corsOrigins,
		logger:      logger,
	}
	server.router = server.routes()
	return server
}

// Run starts the HTTP server and blocks until the context is canceled.
func (s *Server) Run(ctx context.Context) error {
	httpServer := &http.Server{
		Addr:              s.addr,
		Handler:           s.router,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("shutting down", "error", err)
		}
	}()

	s.logger.Debug("starting server", "addr", s.addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	s.logger.Info("server stopped")
	return nil
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()

	router.Use(middleware.Heartbeat("/api/ping"))
	router.Use(middleware.RealIP)
	router.Use(middleware.RequestID)
	router.Use(middleware.CleanPath)
	router.Use(middleware.StripSlashes)
	router.Use(s.requestLoggerMiddleware)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Compress(gzipCompressionLevel))
	router.Use(middleware.Throttle(maxConcurrentRequests))
	router.Use(middleware.Timeout(requestTimeout))
	router.Use(middleware.GetHead)
	router.Use(middleware.SetHeader("X-Content-Type-Options", "nosniff"))

	if len(s.corsOrigins) > 0 {
		router.Use(cors.Handler(cors.Options{
			AllowedOrigins:   s.corsOrigins,
			AllowedMethods:   []string{"GET", "HEAD", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Content-Type"},
			ExposedHeaders:   []string{"Content-Length"},
			AllowCredentials: false,
			MaxAge:           corsMaxAge,
		}))
	}

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Get("/files", s.handleGetFiles)
		apiRouter.Get("/files/{hash}", s.handleGetFile)
		apiRouter.Get("/files/{hash}/raw", s.handleGetFileRaw)
	})

	return router
}

func (s *Server) requestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		wrappedRes := middleware.NewWrapResponseWriter(res, req.ProtoMajor)
		start := time.Now()

		next.ServeHTTP(wrappedRes, req)

		s.logger.Info("request",
			"method", req.Method,
			"path", req.URL.Path,
			"status", wrappedRes.Status(),
			"bytes", wrappedRes.BytesWritten(),
			"duration", time.Since(start),
			"ip", req.RemoteAddr,
			"request_id", middleware.GetReqID(req.Context()),
		)
	})
}

var fieldAccessors = map[db.QueryField]func(*source.FileInfo) any{
	db.FieldHash:       func(file *source.FileInfo) any { return file.Hash },
	db.FieldPath:       func(file *source.FileInfo) any { return file.Path },
	db.FieldDirs:       func(file *source.FileInfo) any { return file.Dirs },
	db.FieldName:       func(file *source.FileInfo) any { return file.Name },
	db.FieldBaseName:   func(file *source.FileInfo) any { return file.BaseName },
	db.FieldExt:        func(file *source.FileInfo) any { return file.Ext },
	db.FieldSize:       func(file *source.FileInfo) any { return file.Size },
	db.FieldModifiedAt: func(file *source.FileInfo) any { return file.ModifiedAt },
	db.FieldMime:       func(file *source.FileInfo) any { return file.Mime },
}

func projectFields(query *db.FileQuery, file *source.FileInfo) map[string]any {
	allFields := lo.Uniq(append([]db.QueryField{db.FieldHash}, query.Fields()...))
	result := make(map[string]any, len(allFields)+1)
	for _, field := range allFields {
		if accessor, ok := fieldAccessors[field]; ok {
			result[field.Column()] = accessor(file)
		}
	}
	result["links"] = newFileLinks(file.Hash)
	return result
}

func (s *Server) handleGetFiles(res http.ResponseWriter, req *http.Request) {
	query, err := parseFileQuery(req.URL.Query())
	if err != nil {
		if validationError, ok := errors.AsType[*queryValidationError](err); ok {
			s.renderError(res, req, http.StatusBadRequest, validationError.Message, nil)
		} else {
			s.renderInternalError(res, req, "parsing query", err)
		}
		return
	}

	files, total, err := s.db.Find(req.Context(), query)
	if err != nil {
		s.renderInternalError(res, req, "querying files", err, "query", req.URL.RawQuery)
		return
	}

	paging := query.Paging()
	meta := meta{
		Skip:  paging.Skip,
		Limit: paging.Limit,
		Total: total,
	}

	if query.HasFieldSelection() {
		projected := make([]map[string]any, len(files))
		for i, file := range files {
			projected[i] = projectFields(query, file)
		}
		resp := dataResponse[any]{Meta: meta, Data: projected}
		s.renderData(res, req, &resp)
		return
	}

	wrapped := make([]fileWithLinks, len(files))
	for i, file := range files {
		wrapped[i] = fileWithLinks{FileInfo: file, Links: newFileLinks(file.Hash)}
	}
	resp := dataResponse[[]fileWithLinks]{Meta: meta, Data: wrapped}
	s.renderData(res, req, &resp)
}

func (s *Server) handleGetFile(res http.ResponseWriter, req *http.Request) {
	file, ok := s.findFileByHash(res, req)
	if !ok {
		return
	}

	resp := dataResponse[fileWithLinks]{
		Meta: meta{Skip: 0, Limit: 1, Total: 1},
		Data: fileWithLinks{FileInfo: file, Links: newFileLinks(file.Hash)},
	}
	s.renderData(res, req, &resp)
}

func (s *Server) handleGetFileRaw(res http.ResponseWriter, req *http.Request) {
	file, ok := s.findFileByHash(res, req)
	if !ok {
		return
	}

	rawURL, err := s.source.RawURL(req.Context(), file)
	if err != nil {
		s.renderInternalError(res, req, "generating raw URL", err, "hash", file.Hash)
		return
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		s.renderInternalError(res, req, "parsing raw URL", err, "url", rawURL)
		return
	}

	if parsed.Scheme == "file" {
		info, err := os.Stat(parsed.Path)
		if err != nil || info.IsDir() {
			s.renderError(res, req, http.StatusNotFound, "not found", map[string]string{"hash": file.Hash})
			return
		}
		http.ServeFile(res, req, parsed.Path)
		return
	}

	http.Redirect(res, req, rawURL, http.StatusTemporaryRedirect)
}

func (s *Server) findFileByHash(res http.ResponseWriter, req *http.Request) (*source.FileInfo, bool) {
	hash := chi.URLParam(req, "hash")
	file, ok, err := s.db.FindByHash(req.Context(), hash)

	if err != nil {
		s.renderInternalError(res, req, "finding file by hash", err, "hash", hash)
		return nil, false
	}

	if !ok {
		s.renderError(res, req, http.StatusNotFound, "not found", map[string]string{"hash": hash})
		return nil, false
	}

	return file, true
}

func (s *Server) renderData(res http.ResponseWriter, req *http.Request, renderer render.Renderer) {
	if err := render.Render(res, req, renderer); err != nil {
		s.logger.Error("rendering response", "error", err)
	}
}

func (s *Server) renderError(res http.ResponseWriter, req *http.Request, status int, message string, details map[string]string) {
	if err := render.Render(res, req, &errResponse{
		HTTPStatusCode: status,
		Error:          errorDetail{Message: message, Details: details},
	}); err != nil {
		s.logger.Error("rendering error response", "error", err)
	}
}

func (s *Server) renderInternalError(res http.ResponseWriter, req *http.Request, logMessage string, err error, keyValues ...any) {
	s.logger.Error(logMessage, append([]any{"error", err}, keyValues...)...)
	s.renderError(res, req, http.StatusInternalServerError, "internal error", nil)
}
