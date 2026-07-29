package store

// ExecRawForTest executes a raw SQL statement against the database. It is a
// test-only seam (GH #44, registered in the spirit of the
// hubclient.NewWithTimeout timeout seam): the W1 black-box server tests need
// to inject rows no public method can produce — e.g. corrupt params JSON in
// image_param_rules — to cover the rule-read failure degradation branches in
// prober, discovery, and server. This is data preparation, not a mock: the
// tests still exercise real behavior through the HTTP API. Production code
// must never call it.
func (db *DB) ExecRawForTest(query string, args ...interface{}) error {
	_, err := db.conn.Exec(query, args...)
	return err
}
