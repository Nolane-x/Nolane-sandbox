# Wave 19 RED Contract Checkpoint

This checkpoint records the intended pre-implementation contract surface before GitHub Actions is used to capture failing evidence.

The branch intentionally contains no Wave 19 production implementation at this checkpoint. RED tests require:

- PID-reuse-resistant `/proc/<pid>/stat` and cgroup membership inspection;
- controller-local placement/lifecycle tokens and realization binding;
- trusted CubeBox post-`AddProc` recorder ordering;
- exact Prometheus host-process identity transport;
- strict NolaneWorld single-scrape identity correlation;
- explicit guards against OOM-victim classification.

Expected RED failures are undefined Wave 19 types/functions/methods or equivalent missing contract implementation. Unexpected failures caused solely by malformed tests, unsupported Go syntax, unrelated pre-existing tests, or workflow configuration are not accepted as RED evidence and must be corrected before production code is added.

Autonomously-by: ChatGPT:GPT-5.6-Sol
