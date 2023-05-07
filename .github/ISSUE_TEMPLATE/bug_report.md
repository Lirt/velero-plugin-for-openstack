---
name: Bug report
about: Create a report to help us improve
title: "[BUG] ..."
labels: bug
assignees: ''

---

**Describe the bug**
- A clear and concise description of what the bug is.
- Is the bug affecting block storage or volume backup mechanism?

**Steps to reproduce the behavior**
1. ...

**Expected behavior**
A clear and concise description of what you expected to happen.

**Used versions**
* Velero version(`velero version`): ...
* Plugin version(`kubectl describe pod velero-...`): ...
* Kubernetes version(`kubectl version`): ...
* Openstack version: ...

**Link to velero or backup log**
Either link to uploaded log of velero pod(`kubectl logs velero-...`) or link to uploaded file with output of command `velero backup logs <BACKUP_NAME>`.
