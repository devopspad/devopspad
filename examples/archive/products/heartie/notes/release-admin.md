---
title: Releasing heartie admin
refs: [service:heartie/admin]
tags: [release]
---
# Releasing heartie admin

1. Release api first when both change; admin calls the new endpoints.
2. Use DevOpsPad (desktop or devopspadcli) for the image release; check `/healthz` afterwards.
3. Nginx changes: edit `configs/admin/pro/heartie-admin.conf` here first, then apply it through DevOpsPad.
