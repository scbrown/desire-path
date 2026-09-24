# Thin aliases for the existing Make targets.
build:
    make build

test:
    make test integration-test

check:
    python3 scripts/check-md-links.py
    python3 scripts/test-md-links.py
    python3 scripts/check-private-keys.py
    make vet test integration-test docs
