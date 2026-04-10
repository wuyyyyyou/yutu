// Copyright 2026 eat-pray-ai & OpenWaygate
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"os"

	"github.com/eat-pray-ai/yutu/cmd"
	_ "github.com/eat-pray-ai/yutu/cmd/activity"
	_ "github.com/eat-pray-ai/yutu/cmd/agent"
	_ "github.com/eat-pray-ai/yutu/cmd/caption"
	_ "github.com/eat-pray-ai/yutu/cmd/channel"
	_ "github.com/eat-pray-ai/yutu/cmd/channelBanner"
	_ "github.com/eat-pray-ai/yutu/cmd/channelSection"
	_ "github.com/eat-pray-ai/yutu/cmd/comment"
	_ "github.com/eat-pray-ai/yutu/cmd/commentThread"
	_ "github.com/eat-pray-ai/yutu/cmd/i18nLanguage"
	_ "github.com/eat-pray-ai/yutu/cmd/i18nRegion"
	_ "github.com/eat-pray-ai/yutu/cmd/member"
	_ "github.com/eat-pray-ai/yutu/cmd/membershipsLevel"
	_ "github.com/eat-pray-ai/yutu/cmd/playlist"
	_ "github.com/eat-pray-ai/yutu/cmd/playlistImage"
	_ "github.com/eat-pray-ai/yutu/cmd/playlistItem"
	_ "github.com/eat-pray-ai/yutu/cmd/search"
	_ "github.com/eat-pray-ai/yutu/cmd/subscription"
	_ "github.com/eat-pray-ai/yutu/cmd/superChatEvent"
	_ "github.com/eat-pray-ai/yutu/cmd/thumbnail"
	_ "github.com/eat-pray-ai/yutu/cmd/video"
	_ "github.com/eat-pray-ai/yutu/cmd/videoAbuseReportReason"
	_ "github.com/eat-pray-ai/yutu/cmd/videoCategory"
	_ "github.com/eat-pray-ai/yutu/cmd/watermark"
)

func main() {
	if len(os.Args) > 1 {
		cmd.Execute()
		return
	}

	executablePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	if err := cmd.ExecuteExecuta(context.Background(), executablePath, os.Stdin, os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
