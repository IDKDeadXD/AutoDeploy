# Changelog

## 0.2.0

- Improved CLI help with grouped commands, aligned descriptions, and practical examples.
- Standardized command output with section headers, aligned fields, masked webhook secrets, and clear `[ok]` / `[fail]` status markers.
- Added CLI output tests covering help, project tables, status, dry-run previews, and webhook secret masking.
- Escaped the Discord mention sanitizer's zero-width space so static analysis can read the intent.

## 0.1.0

- Initial Linux deployment agent release.
