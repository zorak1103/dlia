Generate a concise executive summary of this Docker log scan.

Analyzed {{.ContainerCount}} container(s):

{{.ContainerAnalyses}}

The severity levels shown with each container and the overall severity line are computed by DLIA — treat them as authoritative; do not reassess them.

Create a brief executive summary (max 250 words) for notification delivery:

1. **Overall Status**: One-line health assessment
2. **Critical Issues**: List most urgent problems (if any)
3. **Action Required**: Yes/No and what action
4. **Affected Containers**: Which containers need attention

Keep it concise and actionable. Focus on what needs immediate attention.
