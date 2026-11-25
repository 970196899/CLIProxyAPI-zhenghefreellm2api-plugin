//go:build live

package workbuddy

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"freetier2api-plugin/internal/core"
)

// TestLiveReadOnlyProbe 是只读协议巡检：逐条打现有读接口，打印成功/失败，
// 用来判断哪些路径已经和最新客户端漂移。不会 accept / claim / draw。
//
//	WORKBUDDY_LIVE=1 WORKBUDDY_LIVE_REALM=global \
//	WORKBUDDY_LIVE_CREDS=/path/to/auth.json \
//	  go test -tags live ./internal/vendors/workbuddy -run TestLiveReadOnlyProbe -v
func TestLiveReadOnlyProbe(t *testing.T) {
	if os.Getenv("WORKBUDDY_LIVE") == "" {
		t.Skip("WORKBUDDY_LIVE 未设置")
	}
	path := os.Getenv("WORKBUDDY_LIVE_CREDS")
	if path == "" {
		t.Skip("WORKBUDDY_LIVE_CREDS 未设置")
	}
	realm := RegionCN
	if os.Getenv("WORKBUDDY_LIVE_REALM") == "global" {
		realm = RegionGlobal
	}
	raw, errRead := os.ReadFile(path)
	if errRead != nil {
		t.Fatalf("read creds: %v", errRead)
	}
	cred, errParse := ParseCredential(raw, realm)
	if errParse != nil {
		t.Fatalf("parse creds: %v", errParse)
	}
	configureWorkBuddyProbeHost(t)
	rt := newRuntime(core.Env{})
	client := NewClient(rt, rt.options(context.Background()))

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	report := func(name string, err error, detail string) {
		if err != nil {
			t.Logf("PROBE %-22s FAIL %v", name, err)
			return
		}
		t.Logf("PROBE %-22s ok   %s", name, detail)
	}

	tasks, errTasks := client.ListTasks(ctx, cred)
	report("growth.list", errTasks, fmt.Sprintf("n=%d", len(tasks)))
	if errTasks == nil {
		for _, task := range tasks {
			t.Logf("   growth task code=%s current=%d target=%d claimed=%v", task.Code, task.Current, task.Target, task.Claimed)
		}
	}

	mpTasks, errMP := client.ListTasksMP(ctx, cred)
	report("growth.list_mp", errMP, fmt.Sprintf("n=%d", len(mpTasks)))

	streak, errStreak := client.GrowthStreak(ctx, cred)
	if errStreak != nil {
		report("growth.streak", errStreak, "")
	} else {
		report("growth.streak", nil, fmt.Sprintf("days=%d makeup=%d tiers=%d", streak.Days, streak.MakeupCards, len(streak.Tiers)))
	}

	missed, errMissed := client.HeatmapYesterdayMissed(ctx, cred)
	report("growth.yesterday", errMissed, fmt.Sprintf("missed=%v", missed))

	buddy, errBuddy := client.BuddyInfo(ctx, cred)
	if errBuddy != nil {
		report("buddy.info", errBuddy, "")
	} else {
		report("buddy.info", nil, fmt.Sprintf("has_buddy=%v", buddy.HasBuddy))
	}

	travel, errTravel := client.TravelStatus(ctx, cred)
	if errTravel != nil {
		report("travel.status", errTravel, "")
	} else {
		report("travel.status", nil, fmt.Sprintf("state=%s record=%d limit=%v", travel.State, travel.RecordID, travel.DailyLimitReached))
	}

	need, errBlackcat := client.BlackcatNeed(ctx, cred)
	report("blackcat.need", errBlackcat, fmt.Sprintf("n=%d", need))

	school, errSchool := client.SchoolTasks(ctx, cred)
	if errSchool != nil {
		report("school.list", errSchool, "")
	} else {
		report("school.list", nil, fmt.Sprintf("n=%d", len(school.Tasks)))
	}

	chances, errChances := client.SchoolChances(ctx, cred)
	report("school.chances", errChances, fmt.Sprintf("n=%d", chances))

	lottery, errLottery := client.LotteryChances(ctx, cred)
	report("lottery.chances", errLottery, fmt.Sprintf("n=%d", lottery))

	checkin, errCheckin := client.CheckinActivityStatus(cred)
	if errCheckin != nil {
		report("checkin.status", errCheckin, "")
	} else {
		report("checkin.status", nil, fmt.Sprintf("active=%v today=%v streak=%d", checkin.Active, checkin.TodayCheckedIn, checkin.StreakDays))
	}

	experts, errExperts := client.MarketExpertList(ctx, cred, "")
	report("market.experts", errExperts, fmt.Sprintf("n=%d", len(experts)))
}
