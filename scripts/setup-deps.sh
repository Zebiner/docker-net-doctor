#!/bin/bash
# Setup script for Docker Network Doctor dependencies

echo "Setting up project dependencies..."

# Core dependencies
go get github.com/spf13/cobra@v1.8.0
go get github.com/fatih/color@v1.16.0
go get github.com/stretchr/testify@v1.8.4

# Docker and related dependencies
go get github.com/docker/docker@v24.0.7+incompatible
go get github.com/docker/go-connections@v0.4.0
go get github.com/docker/go-units@latest
go get github.com/opencontainers/image-spec/specs-go/v1@latest
go get github.com/opencontainers/go-digest@latest
go get github.com/pkg/errors@latest
go get github.com/gogo/protobuf/proto@latest
go get github.com/docker/distribution/reference@latest
go get golang.org/x/net/proxy@latest

# Clean up and verify
go mod tidy
go mod verify

echo "Dependencies setup complete!"
