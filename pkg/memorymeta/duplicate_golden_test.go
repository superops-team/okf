package memorymeta

import (
	"fmt"
	"testing"

	"github.com/superops-team/okf/pkg/lexical"
	"github.com/superops-team/okf/pkg/okf"
)

// mkConcept builds a durable concept for CheckMemory tests. id is written to
// CustomFields["okf_id"] so identity.FromConcept resolves it; filePath sets the
// Ref. body is the markdown body (kept < 500 chars so it is fully used).
func mkConcept(id, ctype, title, body string, tags []string) *okf.Concept {
	return &okf.Concept{
		Type:     ctype,
		Title:    title,
		Content:  body,
		FilePath: "notes/" + id + ".md",
		Tags:     tags,
		CustomFields: map[string]any{
			"okf_id": id,
		},
	}
}

// memoryCheckCorpus is the fixed durable corpus used by the golden suite. It
// mixes note/event/feedback types, English and Chinese content, common
// vocabulary, and code identifiers so the confusion matrix is representative.
func memoryCheckCorpus() []*okf.Concept {
	return []*okf.Concept{
		mkConcept("okf_11111111111111111111111111111111", "note",
			"Redis caching policy",
			"We cache frequent read responses in Redis with a time to live of five minutes. On a cache miss we query Postgres and populate the cache. Invalidation happens on writes. Use random jitter to avoid thundering herds when many keys expire together.",
			nil),
		mkConcept("okf_22222222222222222222222222222222", "note",
			"Postgres migration runbook",
			"Run database migrations inside a transaction. Take a backup before applying schema changes. Never run destructive DROP TABLE without a reviewed rollback plan. Apply one logical change per migration and keep them small.",
			nil),
		mkConcept("okf_33333333333333333333333333333333", "event",
			"Deploy incident 2026-03-12",
			"The canary deploy on Tuesday rolled back after error rates spiked. The root cause was a missing database index on the orders query. We added the index and re-deployed, and error rates returned to baseline within ten minutes.",
			nil),
		mkConcept("okf_44444444444444444444444444444444", "feedback",
			"Prefer small pull requests",
			"Keep pull requests under four hundred lines so reviewers can read them in one sitting. Split large refactors into stacked diffs. Describe the problem first, then the change, and call out risky edits explicitly.",
			nil),
		mkConcept("okf_55555555555555555555555555555555", "note",
			"Go HTTP server timeouts",
			"Always set read, write, and idle timeouts on the HTTP server. Without them a slow client can hold connections open indefinitely. Use context cancellation to propagate shutdown and to stop in-flight handlers cleanly.",
			nil),
		mkConcept("okf_66666666666666666666666666666666", "note",
			"Kubernetes pod restart loop",
			"A pod stuck in CrashLoopBackOff usually means a failing startup probe or a missing secret. Check the logs and the event stream before restarting the deployment. Resource limits that are too low also trigger repeated restarts under load.",
			nil),
		mkConcept("okf_77777777777777777777777777777777", "event",
			"Database failover completed",
			"The primary database failed over to the replica after a storage blip. Promotion finished in forty seconds. Applications reconnected automatically using the DNS endpoint and no data loss was reported by the write path.",
			nil),
		mkConcept("okf_88888888888888888888888888888888", "feedback",
			"Write tests for new behavior",
			"Every bug fix gets a regression test that fails before the fix. New public functions have at least one happy path and one error path test. Do not weaken existing assertions to make CI green.",
			nil),
		mkConcept("okf_99999999999999999999999999999999", "note",
			"Structured logging with zap",
			"Use zap for structured logging in the service layer. Emit one JSON line per request with request id, latency, status, and user id. Avoid string interpolation inside hot paths and log at debug level for verbose internals.",
			nil),
		mkConcept("okf_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "note",
			"Flaky CI tests",
			"Treat flaky tests as bugs. Re-run the failing test in isolation to reproduce. If it depends on wall clock, inject a clock. Keep the build green by quarantining only after marking a tracking issue.",
			nil),
		mkConcept("okf_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "feedback",
			"Use structured data shapes",
			"Prefer typed structs over maps for internal passing. Logs and metrics should carry stable keys. Freeze the public error contract so clients do not parse free text messages.",
			nil),
		mkConcept("okf_cccccccccccccccccccccccccccccccc", "note",
			"TLS certificate rotation",
			"Renew TLS certificates before the thirty day expiry warning. Store the private key in a secret manager and never commit it to the repository. Rotate the cert and the intermediate bundle together and verify the chain.",
			nil),
		mkConcept("okf_dddddddddddddddddddddddddddddddd", "event",
			"Security patch applied",
			"We backported the latest runtime security patch to all production hosts. The reboot window ran overnight and all services came back healthy. A follow-up audit confirmed no hosts are left on the old version.",
			nil),
		mkConcept("okf_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "note",
			"Rate limiting middleware",
			"Limit requests per client with a token bucket in the HTTP middleware. Return a four twenty nine status with a retry after header. Exempt health checks and internal admin traffic from the limiter.",
			nil),
		mkConcept("okf_ffffffffffffffffffffffffffffffff", "feedback",
			"Document public API changes",
			"Update the changelog and the generated client when you change a public endpoint. Deprecate old fields for one release before removing them. Bump the major version only for breaking changes to the wire format.",
			nil),
		mkConcept("okf_12121212121212121212121212121212", "note",
			"Webhook retry policy",
			"Retry failed webhook deliveries with exponential backoff up to five attempts. Deduplicate by delivery id so the receiver can ignore repeats. Send dead letters to a queue for manual inspection after the final attempt.",
			nil),
		mkConcept("okf_34343434343434343434343434343434", "note",
			"数据库备份策略 backup",
			"每天凌晨两点自动备份数据库到对象存储。备份保留三十天，每周做一次恢复演练。备份完成后校验文件完整性，失败时告警值班人员。",
			nil),
		mkConcept("okf_56565656565656565656565656565656", "note",
			"Naming conventions code style",
			"Use mixedCaps for exported functions like ParseConfig and loadConfig internally. Keep names short but unambiguous in small scopes. Avoid hungarian notation and do not abbreviate standard words into cryptic identifiers.",
			nil),
		mkConcept("okf_67676767676767676767676767676767", "note",
			"Cache invalidation patterns",
			"We use cache_invalidation hooks on every write path. The HTTPServer middleware calls cache_invalidation before responding. A stale cache is worse than no cache, so every mutation triggers cache_invalidation synchronously.",
			nil),
		mkConcept("okf_78787878787878787878787878787878", "note",
			"Config loading guide",
			"To parse config from yaml, call load_config at startup. The load_config function validates required fields and returns an error if a key is missing. Never parse config ad-hoc in request handlers.",
			nil),
	}
}

// goldenCase is one labeled memory_check evaluation.
type goldenCase struct {
	name     string
	query    string
	positive bool // true => expected status possible_duplicate
}

// goldenCases labels near-duplicate positives and clearly-distinct negatives.
// Positives are rephrases / partial overlaps / cross-language; negatives use
// common words, unrelated Chinese, or code identifiers.
func goldenCases() []goldenCase {
	return []goldenCase{
		// ---- positives: rephrases that share most content tokens ----
		{"p_redis_rephrase", "Redis caching policy: we cache frequent read responses with a five minute time to live and invalidate on writes using jitter to avoid thundering herds.", true},
		{"p_redis_partial", "Redis cache TTL of five minutes, populate on Postgres miss and invalidate on write.", true},
		{"p_pg_migration", "Run postgres migrations inside a transaction, take a backup first, keep one logical change per migration and keep them small.", true},
		{"p_pg_destructive", "Database migration runbook: never run a destructive drop table without a reviewed rollback plan and a backup.", true},
		{"p_deploy_incident", "Canary deploy rolled back after error rates spiked because of a missing orders database index; added the index and redeployed.", true},
		{"p_pr_size", "Prefer small pull requests under four hundred lines; split large refactors into stacked diffs and call out risky edits.", true},
		{"p_http_timeout", "Go HTTP server must set read write and idle timeouts; use context cancellation to stop handlers cleanly on shutdown.", true},
		{"p_k8s_crashloop", "Pod in crashloopbackoff: check logs and events for a failing startup probe or missing secret before restarting the deployment.", true},
		{"p_failover", "Database failed over to the replica after a storage blip; promotion took forty seconds and apps reconnected via DNS.", true},
		{"p_test_regression", "Every bug fix gets a regression test that fails before the fix; new functions have a happy path and an error path test.", true},
		{"p_zap_logging", "Use zap for structured logging: emit one json line per request with request id latency status and user id.", true},
		{"p_flaky_tests", "Treat flaky CI tests as bugs; re-run in isolation to reproduce and inject a clock instead of relying on wall clock.", true},
		{"p_typed_structs", "Prefer typed structs over maps for internal passing and keep stable keys for logs and metrics.", true},
		{"p_tls_rotation", "Renew TLS certificates before the thirty day expiry warning and rotate the cert and intermediate bundle together.", true},
		{"p_security_patch", "Backport the latest runtime security patch to production hosts overnight and confirm no hosts stay on the old version.", true},
		{"p_rate_limit", "Limit requests per client with a token bucket in HTTP middleware and return four twenty nine with a retry after header.", true},
		{"p_api_changelog", "Update the changelog and generated client when changing a public endpoint; deprecate old fields for one release.", true},
		{"p_webhook_retry", "Retry failed webhook deliveries with exponential backoff up to five attempts and deduplicate by delivery id.", true},
		// ---- positives: cross-language (mixed corpus) ----
		{"p_cn_backup", "数据库每天凌晨自动备份到对象存储，备份保留三十天，每周做恢复演练并校验文件完整性。", true},
		{"p_cn_backup_partial", "备份保留三十天，每周一次恢复演练，失败时告警值班人员。", true},
		{"p_cross_lang_redis", "Redis cache frequent read responses with TTL and jitter; the 缓存 strategy populates from Postgres on a miss.", true},
		// ---- positives: identifier subword matching (camelCase/snake_case/kebab) ----
		{"p_ident_camel_forward", "parse config from yaml at startup and validate required fields", true},
		{"p_ident_snake_forward", "cache invalidation happens on every write path before responding", true},
		{"p_ident_load_forward", "load config validates required keys and returns error on missing field", true},
		{"p_ident_camel_reverse", "ParseConfig validates required fields and returns error on missing key", true},
		{"p_ident_snake_reverse", "cache_invalidation hooks run on every write path synchronously", true},
		{"p_ident_kebab_forward", "naming conventions use mixed caps for exported functions like parse config; keep names short and avoid hungarian notation", true},
		// ---- negatives: unrelated common words ----
		{"n_common_fox", "the quick brown fox jumps over the lazy dog near a quiet riverbank this morning", false},
		{"n_common_weather", "today the weather is sunny and we should go for a walk in the park after lunch", false},
		{"n_common_system", "the system is working as expected and all dashboards look healthy right now", false},
		{"n_common_music", "the concert last night was loud and the drummer played a very long solo", false},
		{"n_common_recipe", "bake the bread at two hundred degrees for forty minutes and let it cool before slicing", false},
		{"n_common_books", "the library ordered three new novels and the librarian shelved them by the window", false},
		// ---- negatives: unrelated Chinese ----
		{"n_cn_food", "今天晚上我们去吃火锅，点了很多肉和蔬菜，味道非常好。", false},
		{"n_cn_travel", "下个月计划去海边旅行，已经订好了机票和酒店，想看日出。", false},
		{"n_cn_movie", "这部电影的剧情很感人，演员表演出色，我看了两遍还是很喜欢。", false},
		{"n_cn_garden", "院子里的花开了，红色和黄色都很漂亮，邻居来参观的时候拍了照片。", false},
		// ---- negatives: code identifiers that should not match prose notes ----
		{"n_code_ident", "func parseConfig(foo Bar) Baz { return foo.parse(qux) }", false},
		{"n_code_ident2", "myVar := loadConfig(\"x\"); result := myVar.Transform(); if err != nil { panic(err) }", false},
		{"n_code_path", "src/github.com/okf/pkg/lexical/bm25_index.go:42: cannot use s as string", false},
		{"n_code_snippet", "for i := range n { slices.Sort(s[i]); maps.Copy(dst, src) }", false},
		// ---- negatives: single generic words that leak into many docs ----
		{"n_word_system", "system", false},
		{"n_word_data", "data", false},
		{"n_word_request", "request", false},
		// ---- negatives: near-miss but distinct topics ----
		{"n_distinct_mobile", "Mobile push notifications arrive on the phone even when the app is backgrounded.", false},
		{"n_distinct_pricing", "Pricing for the enterprise tier includes seat based billing and annual contracts only.", false},
		{"n_distinct_payment", "Payment processing uses a third party gateway and tokensize cards so we never store PAN.", false},
		{"n_distinct_search", "Full text search runs in a separate cluster and refreshes its index every five minutes.", false},
		{"n_distinct_cache_cdn", "Static assets are served from a CDN with a long cache control header and purged on deploy.", false},
		{"n_distinct_email", "Transactional emails are sent from a dedicated provider with bounce tracking enabled.", false},
	}
}

// TestCheckMemoryGolden gates duplicate detection on a labeled set >= 40.
// It prints the per-case audit table and the confusion matrix, then enforces
// Precision >= 0.85, Recall >= 0.70, FPR <= 0.15 at the default threshold.
func TestCheckMemoryGolden(t *testing.T) {
	corpus := memoryCheckCorpus()
	cases := goldenCases()
	if len(cases) < 40 {
		t.Fatalf("golden set has %d cases, need >= 40", len(cases))
	}

	var tp, fp, tn, fn int
	fmt.Printf("\n%-26s %-9s %-9s %-9s %-8s\n", "case", "expected", "predicted", "status", "score")
	for _, tc := range cases {
		res := CheckMemory(corpus, tc.query, "", "", "", 0)
		predicted := res.Status == MemoryStatusPossibleDuplicate
		topScore := 0.0
		if len(res.Candidates) > 0 {
			topScore = res.Candidates[0].JaccardScore
		}
		exp := "neg"
		if tc.positive {
			exp = "pos"
		}
		pred := "neg"
		if predicted {
			pred = "pos"
		}
		switch {
		case tc.positive && predicted:
			tp++
		case tc.positive && !predicted:
			fn++
		case !tc.positive && predicted:
			fp++
		case !tc.positive && !predicted:
			tn++
		}
		fmt.Printf("%-26s %-9s %-9s %-9s %.3f\n", tc.name, exp, pred, res.Status, topScore)
	}

	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}
	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}
	fpr := 0.0
	if fp+tn > 0 {
		fpr = float64(fp) / float64(fp+tn)
	}

	fmt.Printf("\nConfusion matrix: TP=%d FP=%d TN=%d FN=%d\n", tp, fp, tn, fn)
	fmt.Printf("Precision=%.3f Recall=%.3f FPR=%.3f\n", precision, recall, fpr)

	if precision < 0.85 {
		t.Errorf("Precision %.3f below gate 0.85 (TP=%d FP=%d)", precision, tp, fp)
	}
	if recall < 0.70 {
		t.Errorf("Recall %.3f below gate 0.70 (TP=%d FN=%d)", recall, tp, fn)
	}
	if fpr > 0.15 {
		t.Errorf("FPR %.3f above gate 0.15 (FP=%d TN=%d)", fpr, fp, tn)
	}
}

// TestCheckMemoryNoSimilar ensures a novel query with no lexical overlap is
// classified no_similar and carries no candidates (S20).
func TestCheckMemoryNoSimilar(t *testing.T) {
	corpus := memoryCheckCorpus()
	res := CheckMemory(corpus, "quantum coffee machines for spaceship cabins", "", "", "", 0)
	if res.Status != MemoryStatusNoSimilar {
		t.Fatalf("expected no_similar, got %s", res.Status)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %d", len(res.Candidates))
	}
}

// TestCheckMemoryPossibleDuplicate ensures a near-duplicate query returns
// possible_duplicate with up to three candidates (S21).
func TestCheckMemoryPossibleDuplicate(t *testing.T) {
	corpus := memoryCheckCorpus()
	res := CheckMemory(corpus,
		"Redis caching policy: we cache frequent read responses with a five minute time to live and invalidate on writes.",
		"", "", "", 0)
	if res.Status != MemoryStatusPossibleDuplicate {
		t.Fatalf("expected possible_duplicate, got %s", res.Status)
	}
	if len(res.Candidates) == 0 || len(res.Candidates) > 3 {
		t.Fatalf("expected 1..3 candidates, got %d", len(res.Candidates))
	}
	if res.Candidates[0].Ref == "" {
		t.Fatalf("expected non-empty ref")
	}
	if res.Candidates[0].OKFID != "okf_11111111111111111111111111111111" {
		t.Fatalf("expected redis note as top candidate, got %s", res.Candidates[0].OKFID)
	}
}

// TestCheckMemoryDeterministic ensures two identical calls are identical (S25).
func TestCheckMemoryDeterministic(t *testing.T) {
	corpus := memoryCheckCorpus()
	q := "Redis caching policy with a five minute time to live"
	a := CheckMemory(corpus, q, "", "", "", 0)
	b := CheckMemory(corpus, q, "", "", "", 0)
	if a.Status != b.Status || len(a.Candidates) != len(b.Candidates) {
		t.Fatalf("non-deterministic: %+v vs %+v", a, b)
	}
	for i := range a.Candidates {
		if a.Candidates[i] != b.Candidates[i] {
			t.Fatalf("non-deterministic candidate %d: %+v vs %+v", i, a.Candidates[i], b.Candidates[i])
		}
	}
}

// TestCheckMemoryThresholdConfigurable verifies S27: lowering the threshold
// promotes a borderline Jaccard to duplicate; raising it suppresses it.
func TestCheckMemoryThresholdConfigurable(t *testing.T) {
	corpus := memoryCheckCorpus()
	// This query shares a few tokens with the redis note but not a full rephrase.
	q := "Redis cache TTL"
	low := CheckMemory(corpus, q, "", "", "", 0)
	_ = low
	// A deliberately low threshold must be possible_duplicate.
	lo := CheckMemory(corpus, q, "", "", "", 0.05)
	hi := CheckMemory(corpus, q, "", "", "", 0.90)
	if lo.Status == MemoryStatusNoSimilar {
		t.Fatalf("low threshold 0.05 should find a similar concept for %q, got no_similar (candidates=%+v)", q, lo.Candidates)
	}
	if hi.Status != MemoryStatusNoSimilar {
		t.Fatalf("high threshold 0.90 should be no_similar, got %s", hi.Status)
	}
}

// TestCheckMemoryTypeFilter verifies that an explicit typeFilter limits the
// candidate set to that single type (S22).
func TestCheckMemoryTypeFilter(t *testing.T) {
	corpus := memoryCheckCorpus()
	// Query that strongly matches a note (redis) but also references events.
	res := CheckMemory(corpus, "Redis caching policy five minute time to live", "event", "", "", 0)
	// No event concept is about redis caching, so no_similar expected.
	if res.Status != MemoryStatusNoSimilar {
		t.Fatalf("typeFilter=event should exclude the redis note, got %s (%+v)", res.Status, res.Candidates)
	}
	if len(res.CandidateTypes) != 1 || res.CandidateTypes[0] != "event" {
		t.Fatalf("expected CandidateTypes=[event], got %v", res.CandidateTypes)
	}
}

// TestCheckMemoryDefaultTypes verifies default candidate types are note/event/feedback.
func TestCheckMemoryDefaultTypes(t *testing.T) {
	corpus := memoryCheckCorpus()
	res := CheckMemory(corpus, "Redis caching policy", "", "", "", 0)
	if len(res.CandidateTypes) != 3 {
		t.Fatalf("expected 3 default candidate types, got %v", res.CandidateTypes)
	}
}

// TestCheckMemoryEmptyContent verifies empty query yields no_similar (no panic).
func TestCheckMemoryEmptyContent(t *testing.T) {
	corpus := memoryCheckCorpus()
	res := CheckMemory(corpus, "", "", "", "", 0)
	if res.Status != MemoryStatusNoSimilar {
		t.Fatalf("empty query should be no_similar, got %s", res.Status)
	}
}

// TestCheckMemoryReadOnly ensures CheckMemory does not mutate input concepts.
func TestCheckMemoryReadOnly(t *testing.T) {
	corpus := memoryCheckCorpus()
	before := fmt.Sprintf("%v", corpus[0].CustomFields)
	_ = CheckMemory(corpus, "Redis caching policy", "", "", "", 0)
	after := fmt.Sprintf("%v", corpus[0].CustomFields)
	if before != after {
		t.Fatalf("CheckMemory mutated concept CustomFields: %q -> %q", before, after)
	}
}

// TestCheckMemoryJaccardTokenizer verifies the Jaccard token rules directly.
func TestCheckMemoryJaccardTokenizer(t *testing.T) {
	// Unicode lowercase.
	toks := jaccardTokens("Hello World")
	joined := fmt.Sprintf("%v", toks)
	if joined != "[hello world]" {
		t.Fatalf("expected [hello world], got %s", joined)
	}
	// Han per-character.
	toks = jaccardTokens("数据库")
	if fmt.Sprintf("%v", toks) != "[数 据 库]" {
		t.Fatalf("expected per-char Han, got %v", toks)
	}
	// Identifier with hyphen and underscore stays one token.
	toks = jaccardTokens("okf-semantic_search")
	if fmt.Sprintf("%v", toks) != "[okf-semantic_search]" {
		t.Fatalf("expected single identifier token, got %v", toks)
	}
}

// TestCheckMemoryJaccardValue verifies set Jaccard math.
func TestCheckMemoryJaccardValue(t *testing.T) {
	if got := jaccardSimilarity([]string{"a", "b", "c"}, []string{"a", "b", "c"}); got != 1.0 {
		t.Fatalf("identical sets should be 1.0, got %f", got)
	}
	if got := jaccardSimilarity(nil, []string{"a"}); got != 0.0 {
		t.Fatalf("empty query set should be 0.0, got %f", got)
	}
	// {a,b,c} vs {a,b,d} -> inter 2, union 4 -> 0.5
	if got := jaccardSimilarity([]string{"a", "b", "c"}, []string{"a", "b", "d"}); got != 0.5 {
		t.Fatalf("expected 0.5, got %f", got)
	}
}

// TestCheckMemoryProjectTagFilter verifies project and tag narrowing.
func TestCheckMemoryProjectTagFilter(t *testing.T) {
	corpus := memoryCheckCorpus()
	// Add a tagged, project-scoped note.
	withTag := mkConcept("okf_77777777777777777777777777777778", "note",
		"Redis caching policy team alpha",
		"We cache frequent read responses in Redis with a time to live of five minutes.",
		[]string{"infra"})
	withTag.CustomFields["project"] = "alpha"
	corpus = append(corpus, withTag)

	// Same query but restricted to project=beta should NOT match the alpha note.
	// (It may still match the base redis note; we just verify no panic and that
	// project filtering drops the project-scoped concept when mismatched.)
	res := CheckMemory(corpus, "Redis caching policy five minute TTL", "", "beta", "", 0)
	for _, c := range res.Candidates {
		if c.OKFID == "okf_77777777777777777777777777777778" {
			t.Fatalf("project=beta should exclude the alpha note, but it appears: %+v", c)
		}
	}
}

// TestCheckMemoryCandidateTypesDefensiveCopy verifies that the returned
// CandidateTypes slice is a defensive copy: mutating it does not affect the
// global defaultDurableTypes or subsequent calls. This guards against the
// regression where CandidateTypes was set to the global slice directly.
func TestCheckMemoryCandidateTypesDefensiveCopy(t *testing.T) {
	corpus := memoryCheckCorpus()

	// First call: default types.
	r1 := CheckMemory(corpus, "test", "", "", "", 0)
	if len(r1.CandidateTypes) != 3 || r1.CandidateTypes[0] != "note" {
		t.Fatalf("first call CandidateTypes = %v, want [note event feedback]", r1.CandidateTypes)
	}

	// Mutate the returned slice — must not pollute the global or later calls.
	r1.CandidateTypes[0] = "MUTATED"
	r1.CandidateTypes = append(r1.CandidateTypes, "extra")

	// Second call: must still return the pristine defaults.
	r2 := CheckMemory(corpus, "test", "", "", "", 0)
	if len(r2.CandidateTypes) != 3 {
		t.Fatalf("after mutation, CandidateTypes len = %d, want 3", len(r2.CandidateTypes))
	}
	if r2.CandidateTypes[0] != "note" || r2.CandidateTypes[1] != "event" || r2.CandidateTypes[2] != "feedback" {
		t.Fatalf("after mutation, CandidateTypes = %v, want [note event feedback]", r2.CandidateTypes)
	}

	// Explicit type filter also returns a defensive copy.
	r3 := CheckMemory(corpus, "test", "note", "", "", 0)
	if len(r3.CandidateTypes) != 1 || r3.CandidateTypes[0] != "note" {
		t.Fatalf("type-filtered CandidateTypes = %v, want [note]", r3.CandidateTypes)
	}
	r3.CandidateTypes[0] = "MUTATED2"
	r4 := CheckMemory(corpus, "test", "note", "", "", 0)
	if r4.CandidateTypes[0] != "note" {
		t.Fatalf("after type-filter mutation, CandidateTypes[0] = %q, want note", r4.CandidateTypes[0])
	}
}

// TestTokenizeBM25FreqParity verifies that tokenizeBM25Freq produces the same
// term frequencies and document length as lexical.Tokenize for representative
// inputs: ASCII prose, CJK, camelCase, snake_case, kebab-case, acronyms, and
// mixed content. This guards against the regression where the custom tokenizer
// skipped splitIdentifier subword expansion, causing document/query token
// asymmetry and reduced recall.
func TestTokenizeBM25FreqParity(t *testing.T) {
	inputs := []string{
		"hello world",
		"ParseConfig loads the config",
		"load_config from yaml",
		"cache-invalidation on write",
		"HTTPServer timeout settings",
		"my_variable_name is set",
		"数据库备份策略每天凌晨执行",
		"Use ParseConfig and load_config together",
		"a_b-c.d mixed delimiters",
		"",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			// Reference: lexical.Tokenize builds a frequency map.
			refTokens := lexical.Tokenize(in)
			refFreq := make(map[string]int, len(refTokens))
			for _, tok := range refTokens {
				refFreq[tok]++
			}

			// Under test: streaming tokenizer with request-scoped interner.
			gotFreq := make(map[string]int, len(refTokens))
			interner := make(map[string]string, len(refTokens))
			gotLen := tokenizeBM25Freq(in, gotFreq, interner)

			if gotLen != len(refTokens) {
				t.Errorf("docLen = %d, want %d (ref tokens: %v)", gotLen, len(refTokens), refTokens)
			}
			if len(gotFreq) != len(refFreq) {
				t.Errorf("distinct terms = %d, want %d\ngot: %v\nwant: %v", len(gotFreq), len(refFreq), gotFreq, refFreq)
			}
			for tok, wantCount := range refFreq {
				if gotFreq[tok] != wantCount {
					t.Errorf("term %q: got count %d, want %d", tok, gotFreq[tok], wantCount)
				}
			}
			for tok := range gotFreq {
				if _, ok := refFreq[tok]; !ok {
					t.Errorf("extra term %q not in reference", tok)
				}
			}
		})
	}
}
