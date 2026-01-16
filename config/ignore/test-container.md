# Ignore Instructions for Test Container

Please ignore the following patterns in the logs:

1. **Debug Messages**: Any log lines containing "DEBUG" should be treated as informational only and not flagged as issues.
2. **Known Warnings**: Warnings about "deprecated API" are expected during migration and should be ignored.
3. **Test Errors**: Any errors containing "TEST_IGNORE" are from development tests and should not be reported.

Apply semantic understanding to these rules - the goal is to filter noise while still catching genuine problems.
