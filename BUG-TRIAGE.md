# Jaeger Bug Triage

This procedure describes the steps project maintainers and approvers should take to triage a bug report.

Bugs should be created using the [Bug Report](https://github.com/jaegertracing/jaeger/issues/new?assignees=&labels=bug&projects=&template=bug_report.yaml&title=%5BBug%5D%3A+) issue template.
This template automatically applies the `bug` and `triage` labels.

## Gather Required Information

The first step is to ensure the bug report is unique and complete.
If the bug report is not unique, leave a comment with a link to the existing bug and close the issue.
If the bug report is not complete, leave a comment requesting additional details and apply the `needs-info` label.
When the user provides additional details, remove the `needs-info` label and repeat this step.

## Categorize

Once a bug report is complete, we can determine if it is truly a bug or not.
A bug is defined as code that does not work the way it was intended to work when written.
A change required by specification may not be a bug if the code is working as intended.
If a bug report is determined not to be a bug, remove the `bug` label and apply the appropriate labels as follows:

- `documentation` feature is working as intended but documentation is incorrect or incomplete
- `enhancement` new feature request 

If there is a bug report which is a simple fix these could be tagged with the following labels.
- `good first issue` this is a good issue for a new contributor to tackle to get comfortable with our project
- `help wanted` these are features that the maintainers would like help on due to time constraints

## Triage Complete

The final step is to remove the `triage` label.
This indicates that all above steps have been taken and an issue is ready to be worked on.