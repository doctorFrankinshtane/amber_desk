package sherlock

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const SupportedVersion = "0.16.0"

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

type RunnerStatus struct {
	Ready   bool   `json:"ready"`
	Version string `json:"version,omitempty"`
	Message string `json:"message"`
}

type ScanRequest struct {
	Username string
}

type Result struct {
	ID             string `json:"id"`
	Site           string `json:"site"`
	ProfileURL     string `json:"profileUrl"`
	MainURL        string `json:"mainUrl,omitempty"`
	Status         string `json:"status"`
	HTTPStatus     int    `json:"httpStatus,omitempty"`
	ResponseTimeMS int64  `json:"responseTimeMs,omitempty"`
}

type ScanReport struct {
	Username string   `json:"username"`
	Results  []Result `json:"results"`
}

type Event struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Checked int    `json:"checked,omitempty"`
	Claimed int    `json:"claimed,omitempty"`
	Errors  int    `json:"errors,omitempty"`
}

type EventSink func(Event)

type Runner interface {
	Status(context.Context) RunnerStatus
	Run(context.Context, ScanRequest, EventSink) (ScanReport, error)
}

type CLIRunner struct {
	Python  string
	Timeout time.Duration
}

func NewCLIRunner(python string) *CLIRunner {
	if python == "" {
		python = defaultPython()
	}
	return &CLIRunner{Python: python, Timeout: 30 * time.Second}
}

func defaultPython() string {
	if os.PathSeparator == '\\' {
		return filepath.Join(".tools", "sherlock", "Scripts", "python.exe")
	}
	return filepath.Join(".tools", "sherlock", "bin", "python")
}

func (r *CLIRunner) Status(ctx context.Context) RunnerStatus {
	python, err := filepath.Abs(r.Python)
	if err != nil {
		return RunnerStatus{Message: "invalid SHERLOCK_PYTHON path"}
	}
	if info, statErr := os.Stat(python); statErr != nil || info.IsDir() {
		return RunnerStatus{Message: "Sherlock runtime is not installed; run the local setup script"}
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(checkCtx, python, "-c", "import importlib.metadata; print(importlib.metadata.version('sherlock-project'))").Output()
	if err != nil {
		return RunnerStatus{Message: "Sherlock runtime could not be inspected"}
	}
	version := strings.TrimSpace(string(out))
	if version != SupportedVersion {
		return RunnerStatus{Version: version, Message: fmt.Sprintf("Sherlock %s is required; found %s", SupportedVersion, version)}
	}
	return RunnerStatus{Ready: true, Version: version, Message: "Sherlock is ready"}
}

func (r *CLIRunner) Run(ctx context.Context, request ScanRequest, emit EventSink) (ScanReport, error) {
	if !ValidUsername(request.Username) {
		return ScanReport{}, errors.New("invalid username")
	}
	status := r.Status(ctx)
	if !status.Ready {
		return ScanReport{}, errors.New(status.Message)
	}
	tempRoot := filepath.Join(os.TempDir(), "amberdesk-sherlock")
	if err := os.MkdirAll(tempRoot, 0700); err != nil {
		return ScanReport{}, fmt.Errorf("create scan directory: %w", err)
	}
	directory, err := os.MkdirTemp(tempRoot, "scan-")
	if err != nil {
		return ScanReport{}, fmt.Errorf("create scan directory: %w", err)
	}
	defer os.RemoveAll(directory)

	python, _ := filepath.Abs(r.Python)
	timeout := int(r.Timeout.Seconds())
	if timeout < 1 {
		timeout = 30
	}
	cmd := exec.CommandContext(ctx, python, "-m", "sherlock_project", "--local", "--csv", "--print-all", "--no-color", "--timeout", strconv.Itoa(timeout), "--folderoutput", directory, request.Username)
	cmd.Dir = directory
	cmd.Env = boundedEnvironment()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return ScanReport{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return ScanReport{}, err
	}
	if err := cmd.Start(); err != nil {
		return ScanReport{}, fmt.Errorf("start Sherlock: %w", err)
	}

	var stderrText strings.Builder
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&stderrText, io.LimitReader(stderr, 32<<10))
		_, _ = io.Copy(io.Discard, stderr)
		close(done)
	}()
	scanProgress(stdout, emit)
	waitErr := cmd.Wait()
	<-done
	if waitErr != nil {
		if ctx.Err() != nil {
			return ScanReport{}, ctx.Err()
		}
		message := strings.TrimSpace(stderrText.String())
		if message == "" {
			message = waitErr.Error()
		}
		return ScanReport{}, fmt.Errorf("Sherlock failed: %s", limited(message, 500))
	}

	path, err := firstCSV(directory)
	if err != nil {
		return ScanReport{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return ScanReport{}, fmt.Errorf("open Sherlock report: %w", err)
	}
	defer file.Close()
	return ParseCSV(file, request.Username)
}

func ValidUsername(value string) bool { return usernamePattern.MatchString(value) }

func boundedEnvironment() []string {
	allowed := map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "TEMP": true, "TMP": true, "HOME": true, "USERPROFILE": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true}
	result := make([]string, 0, len(allowed))
	for _, item := range os.Environ() {
		name, _, ok := strings.Cut(item, "=")
		if ok && allowed[strings.ToUpper(name)] {
			result = append(result, item)
		}
	}
	return result
}

func scanProgress(reader io.Reader, emit EventSink) {
	if emit == nil {
		emit = func(Event) {}
	}
	checked, claimed, failures := 0, 0, 0
	scanner := bufio.NewScanner(io.LimitReader(reader, 2<<20))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := limited(strings.TrimSpace(scanner.Text()), 500)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "[+]"):
			checked++
			claimed++
		case strings.HasPrefix(line, "[-]"):
			checked++
			if !strings.Contains(line, "Not Found!") {
				failures++
			}
		}
		emit(Event{Type: "progress", Message: line, Checked: checked, Claimed: claimed, Errors: failures})
	}
}

func firstCSV(directory string) (string, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read Sherlock report directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".csv") {
			return filepath.Join(directory, entry.Name()), nil
		}
	}
	return "", errors.New("Sherlock did not produce a CSV report")
}

func ParseCSV(reader io.Reader, username string) (ScanReport, error) {
	csvReader := csv.NewReader(io.LimitReader(reader, 4<<20))
	csvReader.FieldsPerRecord = -1
	records, err := csvReader.ReadAll()
	if err != nil || len(records) == 0 {
		return ScanReport{}, errors.New("invalid Sherlock CSV report")
	}
	if len(records) > 1001 {
		return ScanReport{}, errors.New("Sherlock CSV report exceeds 1000 results")
	}
	columns := make(map[string]int, len(records[0]))
	for index, name := range records[0] {
		columns[strings.TrimSpace(name)] = index
	}
	for _, required := range []string{"name", "url_main", "url_user", "exists", "http_status", "response_time_s"} {
		if _, ok := columns[required]; !ok {
			return ScanReport{}, fmt.Errorf("Sherlock CSV is missing %s", required)
		}
	}
	results := make([]Result, 0, len(records)-1)
	for _, record := range records[1:] {
		field := func(name string) string {
			index := columns[name]
			if index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		status := normalizeStatus(field("exists"))
		profileURL, err := optionalHTTPURL(field("url_user"))
		if err != nil {
			return ScanReport{}, fmt.Errorf("invalid Sherlock profile URL: %w", err)
		}
		mainURL, err := optionalHTTPURL(field("url_main"))
		if err != nil {
			return ScanReport{}, fmt.Errorf("invalid Sherlock main URL: %w", err)
		}
		if status == "claimed" && profileURL == "" {
			status = "unknown"
		}
		httpStatus, _ := strconv.Atoi(field("http_status"))
		seconds, _ := strconv.ParseFloat(field("response_time_s"), 64)
		site := limited(field("name"), 160)
		hash := sha256.Sum256([]byte(username + "\x00" + site + "\x00" + profileURL))
		results = append(results, Result{ID: hex.EncodeToString(hash[:12]), Site: site, ProfileURL: profileURL, MainURL: mainURL, Status: status, HTTPStatus: httpStatus, ResponseTimeMS: int64(seconds * 1000)})
	}
	return ScanReport{Username: username, Results: results}, nil
}

func optionalHTTPURL(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > 2048 {
		return "", errors.New("URL length is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("only absolute HTTP(S) URLs are allowed")
	}
	return parsed.String(), nil
}

func normalizeStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "claimed", "true":
		return "claimed"
	case "available", "false":
		return "available"
	case "illegal":
		return "illegal"
	case "waf":
		return "waf"
	default:
		return "unknown"
	}
}

func limited(value string, maximum int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maximum {
		return string(runes[:maximum])
	}
	return value
}

type Scan struct {
	ID          string    `json:"id"`
	CaseID      string    `json:"caseId"`
	SourceNode  string    `json:"sourceNodeId"`
	Username    string    `json:"username"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	CompletedAt time.Time `json:"completedAt,omitempty"`
	Checked     int       `json:"checked"`
	Claimed     int       `json:"claimed"`
	Errors      int       `json:"errors"`
	Error       string    `json:"error,omitempty"`
	Results     []Result  `json:"results"`
}

type Manager struct {
	mu       sync.RWMutex
	runner   Runner
	jobs     map[string]*managedScan
	running  int
	shutdown context.Context
}

type managedScan struct {
	scan        Scan
	cancel      context.CancelFunc
	subscribers map[chan Event]struct{}
}

func NewManager(ctx context.Context, runner Runner) *Manager {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Manager{runner: runner, jobs: make(map[string]*managedScan), shutdown: ctx}
}

func (m *Manager) Status(ctx context.Context) RunnerStatus {
	if m == nil || m.runner == nil {
		return RunnerStatus{Message: "Sherlock runner is not configured"}
	}
	return m.runner.Status(ctx)
}

func (m *Manager) Start(caseID, sourceNode, username string) (Scan, error) {
	if !ValidUsername(username) {
		return Scan{}, errors.New("username must contain only letters, digits, dots, underscores, or hyphens")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked(time.Now().UTC())
	for _, job := range m.jobs {
		if job.scan.CaseID == caseID && (job.scan.State == "queued" || job.scan.State == "running") {
			return Scan{}, errors.New("a Sherlock scan is already running for this dossier")
		}
	}
	if m.running >= 2 {
		return Scan{}, errors.New("global Sherlock scan limit reached")
	}
	id := newScanID()
	ctx, cancel := context.WithCancel(m.shutdown)
	job := &managedScan{scan: Scan{ID: id, CaseID: caseID, SourceNode: sourceNode, Username: username, State: "queued", CreatedAt: time.Now().UTC(), Results: []Result{}}, cancel: cancel, subscribers: make(map[chan Event]struct{})}
	m.jobs[id] = job
	m.running++
	go m.run(ctx, job)
	return cloneScan(job.scan), nil
}

func (m *Manager) purgeLocked(now time.Time) {
	for id, job := range m.jobs {
		if (job.scan.State == "completed" || job.scan.State == "cancelled" || job.scan.State == "failed") && now.Sub(job.scan.CompletedAt) > 24*time.Hour {
			delete(m.jobs, id)
		}
	}
	byCase := make(map[string][]*managedScan)
	for _, job := range m.jobs {
		if job.scan.State != "queued" && job.scan.State != "running" {
			byCase[job.scan.CaseID] = append(byCase[job.scan.CaseID], job)
		}
	}
	for _, jobs := range byCase {
		for len(jobs) > 9 {
			oldest := 0
			for index := 1; index < len(jobs); index++ {
				if jobs[index].scan.CreatedAt.Before(jobs[oldest].scan.CreatedAt) {
					oldest = index
				}
			}
			delete(m.jobs, jobs[oldest].scan.ID)
			jobs = append(jobs[:oldest], jobs[oldest+1:]...)
		}
	}
}

func (m *Manager) run(ctx context.Context, job *managedScan) {
	m.mu.Lock()
	job.scan.State = "running"
	job.scan.StartedAt = time.Now().UTC()
	m.publishLocked(job, Event{Type: "progress", Message: "Sherlock scan started"})
	m.mu.Unlock()
	report, err := m.runner.Run(ctx, ScanRequest{Username: job.scan.Username}, func(event Event) {
		m.mu.Lock()
		job.scan.Checked, job.scan.Claimed, job.scan.Errors = event.Checked, event.Claimed, event.Errors
		m.publishLocked(job, event)
		m.mu.Unlock()
	})
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running--
	job.scan.CompletedAt = time.Now().UTC()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			job.scan.State = "cancelled"
			m.publishLocked(job, Event{Type: "cancelled", Message: "Scan cancelled"})
		} else {
			job.scan.State = "failed"
			job.scan.Error = limited(err.Error(), 500)
			m.publishLocked(job, Event{Type: "failed", Message: job.scan.Error})
		}
		return
	}
	job.scan.State = "completed"
	job.scan.Results = report.Results
	job.scan.Checked, job.scan.Claimed, job.scan.Errors = len(report.Results), 0, 0
	for _, result := range report.Results {
		if result.Status == "claimed" {
			job.scan.Claimed++
		} else if result.Status != "available" {
			job.scan.Errors++
		}
	}
	m.publishLocked(job, Event{Type: "completed", Message: "Scan completed", Checked: job.scan.Checked, Claimed: job.scan.Claimed, Errors: job.scan.Errors})
}

func (m *Manager) Get(id string) (Scan, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	job, ok := m.jobs[id]
	if !ok {
		return Scan{}, false
	}
	return cloneScan(job.scan), true
}

func (m *Manager) Cancel(id string) (Scan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Scan{}, errors.New("scan not found")
	}
	if job.scan.State != "queued" && job.scan.State != "running" {
		return Scan{}, errors.New("scan is not running")
	}
	job.cancel()
	return cloneScan(job.scan), nil
}

func (m *Manager) Subscribe(id string) (Scan, <-chan Event, func(), bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[id]
	if !ok {
		return Scan{}, nil, nil, false
	}
	channel := make(chan Event, 16)
	job.subscribers[channel] = struct{}{}
	unsubscribe := func() {
		m.mu.Lock()
		delete(job.subscribers, channel)
		m.mu.Unlock()
	}
	return cloneScan(job.scan), channel, unsubscribe, true
}

func (m *Manager) publishLocked(job *managedScan, event Event) {
	for channel := range job.subscribers {
		select {
		case channel <- event:
		default:
		}
	}
}

func cloneScan(scan Scan) Scan {
	scan.Results = append([]Result(nil), scan.Results...)
	return scan
}

func newScanID() string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())))
	return "SCAN-" + hex.EncodeToString(sum[:8])
}
