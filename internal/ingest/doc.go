// Package ingest implements the source-agnostic ingestion core that normalizes
// inbound items and persists them to Postgres. Scoring is handled out-of-band
// by the decoupled worker; ingestion never scores inline.
package ingest
