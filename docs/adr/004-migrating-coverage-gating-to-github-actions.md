This revised issue description is tailored for an AI agent to implement the "Fan-in" coverage logic using your existing actions/github-script infrastructure.

## ---

**Issue: Move Coverage Gating and PR Reporting from Codecov to GitHub Actions**

### **Context**

Jaeger currently uses Codecov for tracking code coverage trends and gating PRs. Due to latency and reliability issues with Codecov’s status checks, we want to migrate the gating and reporting logic to a local GitHub Action. This will provide faster feedback for PRs while maintaining Codecov as a secondary, long-term historical archive.

### **Requirements**

#### **1\. Update Reusable Coverage Action**

Modify the local reusable action (used across CI jobs) to:

* **Upload Artifacts:** Add an actions/upload-artifact@v4 step to upload the raw .out coverage profile for every test run.  
* **Naming Convention:** Ensure each artifact has a unique name (e.g., coverage-${{ matrix.backend }}-${{ matrix.platform }}) so they can be aggregated.  
* **Keep Codecov:** Maintain the existing Codecov upload logic for long-term trend tracking.

#### **2\. Refactor and Enhance Summary Workflow**

Rename .github/workflows/ci-compare-metrics.yml to .github/workflows/ci-summary-report.yml.

Update this workflow to handle coverage aggregation and gating:

* **Trigger:** Remain on workflow\_run (triggered by the main CI suite) to maintain write permissions for fork PRs.  
* **Artifact Retrieval:** Extend the existing actions/github-script step (which currently fetches metrics) to also download all coverage artifacts (pattern: coverage-\*).  
* **Merge Profiles:** Use gocovmerge to combine the downloaded .out files into a single merged-coverage.out.  
* **Gating Logic:** \* Calculate the total coverage percentage.  
  * Compare it against a baseline (the latest successful main branch coverage).  
  * Fail the job if coverage decreases beyond the allowed threshold (e.g., 0.1%).

#### **3\. Integrated PR Reporting**

* **Generate Markdown:** Use the go-coverage-report CLI to generate a formatted summary table from the merged profile.  
* **Consolidated Comment:** Append this coverage summary to the existing metrics comparison report.  
* **Sticky Comment:** Ensure the marocchino/sticky-pull-request-comment action (or current equivalent) updates the single "Summary Report" comment with both **Performance Metrics** and **Code Coverage** results.

### **Technical Implementation Details**

* **Tools:** gocovmerge, github.com/fgrosse/go-coverage-report/cmd/go-coverage-report@latest.  
* **Permissions:** Ensure pull-requests: write is maintained for the summary job.  
* **Artifact Handling:** Leverage the existing actions/github-script logic for API-based artifact downloads from the triggering workflow.

---

I've streamlined the instructions to focus on extending your current github-script logic. Would you like me to provide a code snippet for the gocovmerge and gating check to include in the issue's "Implementation Hints" section?