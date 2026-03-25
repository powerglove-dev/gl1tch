## Requirements

### Requirement: Steps support for_each iteration over a list
A pipeline step MAY declare a `for_each` field containing a template expression that resolves to a newline-delimited string or a JSON array of strings. The runner SHALL expand the step into N virtual executions (one per item) before DAG construction. Each expansion receives `{{item}}` bound to the current element.

#### Scenario: Step expands for each item
- **WHEN** a step declares `for_each: "{{param.targets}}"` and `param.targets` resolves to `"a\nb\nc"`
- **THEN** the step runs 3 times, once with `{{item}}` = `"a"`, `"b"`, `"c"` respectively

#### Scenario: for_each with JSON array
- **WHEN** `for_each` resolves to a JSON array `["x","y"]`
- **THEN** the step runs twice with `{{item}}` = `"x"` and `"y"`

#### Scenario: for_each with empty list skips step
- **WHEN** `for_each` resolves to an empty string or empty array
- **THEN** the step is marked `skipped` with no executions

### Requirement: for_each outputs collected under step.<id>.items
The runner SHALL store each expansion's output at `step.<id>.items[N].data` where N is the zero-based expansion index. A step that needs a `for_each` step receives all items available in the context once all expansions complete.

#### Scenario: Items indexed in context
- **WHEN** step `scan` runs for_each over 3 items and each returns `{"result": "ok"}`
- **THEN** `step.scan.items[0].data.result`, `step.scan.items[1].data.result`, and `step.scan.items[2].data.result` are all accessible

#### Scenario: Partial failure in for_each
- **WHEN** one expansion of a for_each step fails and others succeed
- **THEN** the overall step is marked `failed` and the specific expansion's index and error are available

### Requirement: for_each respects max_parallel concurrency cap
All expansions of a `for_each` step count toward the pipeline's `max_parallel` limit. The runner SHALL not spawn more than `max_parallel` total goroutines (across all steps) at once, even for `for_each` expansions.

#### Scenario: for_each capped at max_parallel
- **WHEN** a `for_each` step expands to 100 items and `max_parallel` is 8
- **THEN** at most 8 expansion goroutines run simultaneously
