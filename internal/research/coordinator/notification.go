package coordinator

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// NotificationType represents the type of a task notification event.
type NotificationType string

const (
	NotifyPhaseStarted   NotificationType = "phase-started"
	NotifyPhaseCompleted NotificationType = "phase-completed"
	NotifyPhaseFailed    NotificationType = "phase-failed"
	NotifyCheckpoint     NotificationType = "checkpoint"
)

// xmlPhase is the internal XML struct for a phase element.
type xmlPhase struct {
	XMLName xml.Name `xml:"phase"`
	Name    string   `xml:"name,attr"`
	Order   int      `xml:"order,attr"`
	Status  string   `xml:"status,attr"`
}

// xmlNextPhase is the internal XML struct for a next-phase element.
type xmlNextPhase struct {
	XMLName xml.Name `xml:"next-phase"`
	Name    string   `xml:"name,attr"`
	Order   int      `xml:"order,attr"`
}

// xmlNotification is the root XML struct for a task-notification element.
type xmlNotification struct {
	XMLName   xml.Name      `xml:"task-notification"`
	TaskID    string        `xml:"task-id,attr"`
	Type      string        `xml:"type,attr"`
	Phase     xmlPhase      `xml:"phase"`
	NextPhase *xmlNextPhase `xml:"next-phase,omitempty"`
	Summary   string        `xml:"summary"`
}

// FormatPhaseNotification generates an XML task-notification string.
//
// Example output:
//
//	<task-notification task-id="abc" type="phase-completed">
//	  <phase name="Literature Review" order="1" status="completed"/>
//	  <next-phase name="Methodology Design" order="2"/>
//	  <summary>Phase 1 完成，已生成文献综述。</summary>
//	</task-notification>
func FormatPhaseNotification(
	taskID string,
	notifType NotificationType,
	phase PhaseRef,
	nextPhase *PhaseRef,
	summary string,
) string {
	status := notificationTypeToStatus(notifType)

	notif := xmlNotification{
		TaskID: taskID,
		Type:   string(notifType),
		Phase: xmlPhase{
			Name:   phase.Name,
			Order:  phase.Order,
			Status: status,
		},
		Summary: summary,
	}

	if nextPhase != nil {
		notif.NextPhase = &xmlNextPhase{
			Name:  nextPhase.Name,
			Order: nextPhase.Order,
		}
	}

	raw, err := xml.MarshalIndent(notif, "", "  ")
	if err != nil {
		// Fallback: return a minimal error notification.
		return fmt.Sprintf(
			`<task-notification task-id=%q type="error"><summary>failed to marshal notification: %s</summary></task-notification>`,
			taskID, err.Error(),
		)
	}

	// xml.MarshalIndent produces <?xml ...?> header only with EncodeIndent; here we
	// just return the element itself (no XML declaration) as the callers embed it in
	// prose text sent to the Agent.
	return strings.TrimSpace(string(raw))
}

// notificationTypeToStatus maps a NotificationType to a human-readable status string
// suitable for the <phase status="..."/> attribute.
func notificationTypeToStatus(t NotificationType) string {
	switch t {
	case NotifyPhaseStarted:
		return "running"
	case NotifyPhaseCompleted:
		return "completed"
	case NotifyPhaseFailed:
		return "failed"
	case NotifyCheckpoint:
		return "checkpoint"
	default:
		return string(t)
	}
}
