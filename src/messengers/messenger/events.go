package messenger

const (
	// Subject prefix for attachment events
	AttachmentEventsSubject = "events.attachments.>"

	// Subject for registering attachment usage (e.g. events.attachments.register)
	AttachmentRegisterSubject = "events.attachments.register"

	// Subject for processing attachment usage (e.g. events.attachments.processed)
	AttachmentProcessedSubject = "events.attachments.processed"
)

type AttachmentEvent struct {
	BlobID string `json:"blob_id"`
	Delta  int    `json:"delta"` // For registration, delta > 0. For processing, delta = -1.
}
