package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/nexus-edge/protocol-gateway/internal/adapter/mqtt"
)

type TopicRoute struct {
	DeviceID    string `json:"device_id"`
	DeviceName  string `json:"device_name"`
	Protocol    string `json:"protocol"`
	Enabled     bool   `json:"enabled"`
	UNSPrefix   string `json:"uns_prefix"`
	TagID       string `json:"tag_id"`
	TagName     string `json:"tag_name"`
	TopicSuffix string `json:"topic_suffix"`
	FullTopic   string `json:"full_topic"`
	AccessMode  string `json:"access_mode"`
}

type TopicsOverview struct {
	GeneratedAt   time.Time        `json:"generated_at"`
	ActiveTopics  []mqtt.TopicStat `json:"active_topics"`
	Subscriptions []string         `json:"subscriptions"`
	Routes        []TopicRoute     `json:"routes"`
}

// TopicsOverviewHandler returns active topics (recent publishes), subscription patterns,
// and configured routes computed from the current device configuration.
func (h *APIHandler) TopicsOverviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	var active []mqtt.TopicStat
	if h.topicTracker != nil {
		active = h.topicTracker.ActiveTopics(limit)
	} else {
		active = []mqtt.TopicStat{}
	}

	subscriptions := []string{}
	if h.subscriptions != nil {
		subscriptions = h.subscriptions.SubscribedTopics()
		sort.Strings(subscriptions)
	}

	routes := make([]TopicRoute, 0)
	devices := h.deviceManager.GetDevices()
	for _, device := range devices {
		for _, tag := range device.Tags {
			full := device.UNSPrefix
			if tag.TopicSuffix != "" {
				full = full + "/" + tag.TopicSuffix
			}
			routes = append(routes, TopicRoute{
				DeviceID:    device.ID,
				DeviceName:  device.Name,
				Protocol:    string(device.Protocol),
				Enabled:     device.Enabled && tag.Enabled,
				UNSPrefix:   device.UNSPrefix,
				TagID:       tag.ID,
				TagName:     tag.Name,
				TopicSuffix: tag.TopicSuffix,
				FullTopic:   full,
				AccessMode:  string(tag.AccessMode),
			})
		}
	}

	// Sort routes for stable display
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].DeviceID != routes[j].DeviceID {
			return routes[i].DeviceID < routes[j].DeviceID
		}
		return routes[i].FullTopic < routes[j].FullTopic
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(TopicsOverview{
		GeneratedAt:   time.Now(),
		ActiveTopics:  active,
		Subscriptions: subscriptions,
		Routes:        routes,
	})
}

type ContainersResponse struct {
	Containers []string `json:"containers"`
}

// ListContainersHandler returns running container names, if a LogProvider is configured.
func (h *APIHandler) ListContainersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.logProvider == nil {
		http.Error(w, "logs endpoint not configured", http.StatusNotImplemented)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	containers, err := h.logProvider.ListContainers(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	sort.Strings(containers)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ContainersResponse{Containers: containers})
}

type LogsResponse struct {
	Container string     `json:"container"`
	Entries   []LogEntry `json:"entries"`
}

// LogsHandler returns a tail of logs for the selected container.
// Query params: container (required), tail (optional, default 300).
func (h *APIHandler) LogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.logProvider == nil {
		http.Error(w, "logs endpoint not configured", http.StatusNotImplemented)
		return
	}

	container := r.URL.Query().Get("container")
	if container == "" {
		http.Error(w, "container is required", http.StatusBadRequest)
		return
	}

	tail := 300
	if v := r.URL.Query().Get("tail"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			tail = parsed
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	entries, err := h.logProvider.TailLogs(ctx, container, tail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LogsResponse{Container: container, Entries: entries})
}
