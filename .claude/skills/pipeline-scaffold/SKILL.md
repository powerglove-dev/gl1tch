---
name: pipeline-scaffold
description: Scaffold a new .pipeline.yaml with correct step/provider/condition structure for orcai pipelines
disable-model-invocation: true
---

Generate a .pipeline.yaml file for orcai. Steps support:
- input/output types
- provider selection (claude, ollama, copilot, opencode, shell)
- conditions (if/then/else branches)
- plugin execution via gRPC

Reference internal/pipeline/pipeline.go for the Step struct schema.
Reference internal/picker/picker.go for valid provider names from BuildProviders.
Use `./bin/glitch pipeline --help` to verify current flags.

When invoked with arguments, use them as the pipeline name/purpose.
Produce a complete, valid .pipeline.yaml with at least one example step showing provider selection and optional condition branching.
