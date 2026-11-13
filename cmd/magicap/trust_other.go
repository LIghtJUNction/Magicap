//go:build !android

package main

import "context"

func watchAppTrust(ctx context.Context, uids []int) {}
