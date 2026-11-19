package main

import (
	"math"
	"net/http"
	"time"
)

// spans is how long each range is and its bucket, as Sonar serves them.
var spans = map[string][2]time.Duration{
	"1h": {time.Hour, time.Minute}, "24h": {24 * time.Hour, 5 * time.Minute},
	"7d": {7 * 24 * time.Hour, 30 * time.Minute}, "30d": {30 * 24 * time.Hour, 2 * time.Hour},
}

const gib = 1 << 30

// host is Sonar's /v1/monitor/host.
func host(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	sp, ok := spans[rng]
	if !ok {
		http.Error(w, `{"error":"bad range"}`, http.StatusBadRequest)
		return
	}
	span, bucket := sp[0], sp[1]
	now := time.Now().UTC()
	to := now.Truncate(bucket)
	from := to.Add(-span)
	var points []map[string]any
	var eth []map[string]any
	n := int(span / bucket)
	for i := 1; i <= n; i++ {
		t := from.Add(time.Duration(i) * bucket)
		x := float64(t.Unix()) / bucket.Seconds()
		if i > n/3 && i < n/3+n/30+1 { // a gap: the agent was down
			points = append(points, map[string]any{"t": t, "coverage_pct": 0, "cpu_pct": nil, "mem_used_pct": nil})
			continue
		}
		cpu := 16 + 6*math.Sin(x/7) + 3*math.Sin(x/2.3)
		if i == n*3/4 {
			cpu += 38
		}
		mem := 41 + 3*math.Sin(x/23)
		disk := 285e9 + 4e9*float64(i)/float64(n)
		points = append(points, map[string]any{"t": t, "coverage_pct": 100,
			"cpu_pct":        map[string]float64{"avg": cpu, "min": cpu * 0.55, "max": math.Min(100, cpu*1.6)},
			"mem_used_pct":   map[string]float64{"avg": mem, "min": mem - 1.2, "max": mem + 1.5},
			"mem_used_bytes": int64(mem / 100 * 16 * gib), "mem_total_bytes": int64(16 * gib), "swap_used_bytes": int64(322122547),
			"load1":           map[string]float64{"avg": 0.7 + 0.3*math.Sin(x/5), "max": 1.4 + 0.4*math.Sin(x/3)},
			"disk_used_bytes": int64(disk), "disk_total_bytes": int64(468e9)})
		rx := (180e3 + 90e3*math.Sin(x/4) + 40e3*math.Abs(math.Sin(x*1.7))) * bucket.Seconds()
		tx := (60e3 + 30e3*math.Sin(x/6) + 20e3*math.Abs(math.Sin(x*1.3))) * bucket.Seconds()
		if i == n*2/3 {
			rx *= 4
		}
		eth = append(eth, map[string]any{"t": t, "minutes": int(bucket.Minutes()), "rx_bytes": int64(rx), "tx_bytes": int64(tx)})
	}
	cpuNow := 18.4
	cores := []float64{22.1, 14.8, 31.2, 9.6, 17.3, 12.0, 25.4, 15.1}
	booted := now.Add(-(12*24 + 4) * time.Hour)
	updates := []map[string]any{
		{"project": "lys", "started": to.Add(-span * 3 / 4), "ended": to.Add(-span*3/4 + 41*time.Second), "seconds": 41, "from": "v0.5.0", "to": "v0.5.1", "outcome": "ok"},
		{"project": "dibs", "started": to.Add(-span / 4), "ended": to.Add(-span/4 + 2*time.Minute), "seconds": 120, "from": "v0.2.9", "to": "v0.3.0", "outcome": "failed"},
		{"project": "mobasity-web", "started": to.Add(-2 * time.Minute), "ended": to, "seconds": 120, "from": "v1.7.2", "to": "v1.8.0", "outcome": "ok"},
	}
	writeJSON(w, map[string]any{"range": rng, "from": from, "to": to, "bucket_seconds": int(bucket.Seconds()), "generated_at": now,
		"now": map[string]any{
			"read_at": now.Add(-8 * time.Second), "cpu_pct": cpuNow, "core_pcts": cores,
			"memory_total": uint64(16 * gib), "memory_used": uint64(7194070220), "memory_used_pct": 41.9,
			"swap_total": uint64(2 * gib), "swap_used": uint64(322122547),
			"load1": 0.82, "load5": 0.71, "load15": 0.65, "uptime_seconds": now.Sub(booted).Seconds(), "booted_at": booted,
			"disk": map[string]any{"total": uint64(468e9), "used": uint64(289e9), "available": uint64(179e9), "used_pct": 61.75, "inodes": 29e6, "inodes_free": 26e6},
			"network": []map[string]any{
				{"interface": "enp3s0", "rx_bytes_per_s": 212e3, "tx_bytes_per_s": 74e3},
				{"interface": "docker0", "rx_bytes_per_s": nil, "tx_bytes_per_s": nil},
			},
		},
		"points": points, "network": []map[string]any{{"interface": "enp3s0", "points": eth}}, "updates": updates})
}

// containers are the services each project runs: service, image, memory used and limit (MiB), CPU cores.
var containers = map[string][][5]any{
	"blackbox":        {{"blackbox-proxy", "blackbox-blackbox-proxy", 18, 32, 0.002}, {"blackbox-collector", "blackbox-blackbox-collector", 46, 128, 0.031}, {"blackbox-api", "blackbox-blackbox-api", 22, 64, 0.004}},
	"cove":            {{"cove", "cove-cove", 24, 64, 0.003}},
	"dibs":            {{"dibs", "dibs-dibs", 0, 128, 0.0}},
	"lighthouse":      {{"lighthouse", "lighthouse-lighthouse", 38, 256, 0.012}, {"lighthouse-api", "lighthouse-lighthouse-api", 14, 64, 0.001}},
	"lucassariol-web": {{"lucassariol-web", "lucassariol-web-web", 19, 64, 0.002}},
	"lys":             {{"lys", "lys-lys", 142, 512, 0.124}},
	"marquee":         {{"marquee", "marquee-marquee", 31, 128, 0.006}},
	"mobasity-web":    {{"mobasity-web", "mobasity-web-web", 27, 128, 0.009}},
	"plop":            {{"plop", "plop-plop", 58, 256, 0.018}},
	"sonar":           {{"sonar-proxy", "sonar-sonar-proxy", 12, 32, 0.002}, {"sonar-agent", "sonar-sonar-agent", 41, 128, 0.041}, {"sonar-api", "sonar-sonar-api", 29, 128, 0.006}},
	"sparkdb":         {{"sparkdb", "postgres:16.4", 412, 2048, 0.087}},
	"cloudflared":     {{"cloudflared", "cloudflare/cloudflared:2024.10.0", 33, 128, 0.011}},
	"vista":           {{"vista", "vista-vista", 16, 128, 0.002}},
}

var infra = map[string]bool{"sparkdb": true, "cove": true, "lighthouse": true, "cloudflared": true, "sonar": true, "blackbox": true}

func monitorProjects(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	var out []map[string]any
	names := append([]string{"cloudflared", "sparkdb"}, func() []string {
		var n []string
		for _, p := range projects {
			n = append(n, p.name)
		}
		return n
	}()...)
	for i, name := range names {
		state, version, evidence := "operational", "v1.0.0", "container"
		for _, p := range projects {
			if p.name == name {
				state, version, evidence = p.state, p.version, p.evidence
			}
		}
		var services []map[string]any
		for j, c := range containers[name] {
			memMiB, limMiB, cores := c[2].(int), c[3].(int), c[4].(float64)
			started := now.Add(-time.Duration(3+(i*7+j*5)%11) * 24 * time.Hour)
			cs := map[string]any{"name": c[0], "image": c[1], "version": version, "state": "running", "health": "healthy", "exit_code": 0,
				"restart_count": 0, "created_at": started, "started_at": started, "finished_at": nil, "cpu_limit": nil,
				"memory_limit": int64(limMiB) << 20,
				"usage": map[string]any{"cpu_cores": cores, "memory_bytes": uint64(memMiB) << 20,
					"net_rx_bytes_per_s": cores * 9e4, "net_tx_bytes_per_s": cores * 4e4, "blk_read_bytes_per_s": cores * 1e3, "blk_write_bytes_per_s": cores * 5e4}}
			svcState := state
			if name == "dibs" {
				fin := now.Add(-14 * time.Minute)
				cs["state"], cs["health"], cs["exit_code"], cs["restart_count"], cs["finished_at"] = "exited", "none", 1, 5, fin
				cs["usage"] = map[string]any{}
			}
			if name == "lys" {
				cs["restart_count"] = 2
			}
			services = append(services, map[string]any{"service": c[0], "state": svcState, "counted": true, "update": nil,
				"usage": cs["usage"], "containers": []any{cs}})
		}
		var checks []map[string]any
		if evidence == "checked" {
			st, outcome, status, lat := "operational", "ok", 200, int64(40+len(name)*3)
			var errText any
			if state == "down" {
				st, outcome, status, errText = "down", "failed", 502, "502 Bad Gateway"
			}
			checks = append(checks, map[string]any{"service": name, "url": "https://" + name + ".mobasity.com", "state": st, "last_at": now.Add(-20 * time.Second),
				"last_outcome": outcome, "last_status": status, "last_latency_ms": lat, "last_error": errText, "consecutive_failures": map[bool]int{true: 14, false: 0}[state == "down"]})
		}
		var group any
		if infra[name] {
			group = "infrastructure"
		}
		since := now.Add(-time.Duration(30+i*41) * time.Hour)
		if name == "dibs" {
			since = now.Add(-14 * time.Minute)
		}
		out = append(out, map[string]any{"project": name, "group": group, "state": state, "state_since": since, "state_since_exact": true,
			"evidence": evidence, "version": version, "services": services, "checks": checks})
	}
	writeJSON(w, map[string]any{"generated_at": now, "fresh": true, "projects": out})
}

func conditions(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	var collectors []map[string]any
	for _, c := range []string{"docker", "host", "network", "databases", "storage", "cloudflare", "apps"} {
		collectors = append(collectors, map[string]any{"name": c, "state": "available", "last_ok": now.Add(-10 * time.Second), "error": nil})
	}
	writeJSON(w, map[string]any{"generated_at": now, "fresh": true, "state": "warning",
		"conditions": []map[string]any{
			{"id": "swap", "subject": nil, "level": "warning", "message": "Swap is in use: 312 MB of 2.0 GB", "since": now.Add(-3 * time.Hour)},
		},
		"collectors": collectors, "attention": []string{}})
}

func states(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	span := spans[r.URL.Query().Get("range")][0]
	if span == 0 {
		span = 7 * 24 * time.Hour
	}
	from := now.Add(-span)
	trouble := map[string][][3]any{ // state, minutes ago it began, minutes it lasted
		"dibs": {{"down", 14, 14}, {"down", 6 * 1440, 32}},
		"lys":  {{"degraded", 300, 25}, {"down", 3 * 1440, 4}},
		"plop": {{"down", 20 * 1440, 90}},
	}
	var out []map[string]any
	for name, ts := range trouble {
		var sp []map[string]any
		for _, t := range ts {
			start := now.Add(-time.Duration(t[1].(int)) * time.Minute)
			if start.Before(from) {
				continue
			}
			end := start.Add(time.Duration(t[2].(int)) * time.Minute)
			cause := []map[string]any{{"service": name, "state": t[0], "minutes": t[2]}}
			checks := []map[string]any{{"service": name, "url": "https://" + name + ".mobasity.com", "state": t[0], "failures": t[2], "slow": 0, "last_error": "502 Bad Gateway"}}
			sp = append(sp, map[string]any{"state": t[0], "start": start, "end": end, "observed_minutes": t[2], "services": cause, "checks": checks})
		}
		if sp != nil {
			out = append(out, map[string]any{"project": name, "spans": sp})
		}
	}
	writeJSON(w, map[string]any{"range": r.URL.Query().Get("range"), "from": from, "to": now, "generated_at": now, "projects": out})
}

func usage(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	sp, ok := spans[rng]
	if !ok {
		http.Error(w, `{"error":"bad range"}`, http.StatusBadRequest)
		return
	}
	project := r.URL.Query().Get("project")
	to := time.Now().UTC().Truncate(sp[1])
	from := to.Add(-sp[0])
	var out []map[string]any
	for _, c := range containers[project] {
		memMiB, cores := float64(c[2].(int)), c[4].(float64)
		var pts []map[string]any
		n := int(sp[0] / sp[1])
		for i := 1; i <= n; i++ {
			t := from.Add(time.Duration(i) * sp[1])
			x := float64(t.Unix()) / sp[1].Seconds()
			cpu := cores * (1 + 0.5*math.Sin(x/6))
			mem := memMiB * (1 + 0.08*math.Sin(x/17)) * (1 << 20)
			pts = append(pts, map[string]any{"t": t, "minutes": int(sp[1].Minutes()),
				"cpu_cores":    map[string]float64{"avg": cpu, "max": cpu * 1.7},
				"memory_bytes": map[string]int64{"avg": int64(mem), "max": int64(mem * 1.1)},
				"net_rx_bytes": int64(cores * 9e4 * sp[1].Seconds()), "net_tx_bytes": int64(cores * 4e4 * sp[1].Seconds()),
				"blk_read_bytes": int64(cores * 1e3 * sp[1].Seconds()), "blk_write_bytes": int64(cores * 5e4 * sp[1].Seconds())})
		}
		out = append(out, map[string]any{"project": project, "service": c[0], "points": pts})
	}
	writeJSON(w, map[string]any{"range": rng, "from": from, "to": to, "bucket_seconds": int(sp[1].Seconds()), "generated_at": time.Now().UTC(), "services": out})
}

func databases(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	dbs := []map[string]any{}
	total := 0
	for i, name := range []string{"lys", "lighthouse", "sonar", "blackbox", "cove", "dibs", "plop"} {
		conns := []int{6, 3, 4, 5, 2, 0, 1}[i]
		total += conns
		dbs = append(dbs, map[string]any{"name": name + "_db", "project": name, "size_bytes": []int64{182e6, 24e6, 610e6, 905e6, 9e6, 31e6, 12e6}[i],
			"connections": conns, "active": conns / 3, "oldest_xact_secs": nil})
	}
	dbs = append(dbs, map[string]any{"name": "postgres", "project": nil, "size_bytes": 7.5e6, "connections": 1, "active": 1, "oldest_xact_secs": 0})
	writeJSON(w, map[string]any{"range": "1h", "generated_at": now, "databases": []any{},
		"now": map[string]any{"read_at": now.Add(-12 * time.Second), "max_connections": 100, "connections": total + 1, "databases": dbs}})
}

func disk(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	var ps []map[string]any
	var data, images, ctrs, vols int64
	for i, p := range []string{"blackbox", "cove", "dibs", "lighthouse", "lys", "plop", "sonar", "sparkdb", "vista"} {
		d := []int64{0, 2e6, 14e6, 380e6, 26e6, 410e6, 0, 2.1e9, 0}[i]
		im := []int64{42e6, 21e6, 33e6, 61e6, 38e6, 29e6, 47e6, 412e6, 18e6}[i]
		vol := []int64{1.02e9, 0, 0, 0, 0, 0, 0, 0, 0}[i]
		data, images, ctrs, vols = data+d, images+im, ctrs+1e6, vols+vol
		var db any
		if d > 0 {
			db = d
		}
		ps = append(ps, map[string]any{"project": p, "data_bytes": db, "data_files": d / 4096, "data_unreadable": 0, "image_bytes": im, "container_bytes": int64(1e6), "volume_bytes": vol})
	}
	writeJSON(w, map[string]any{"range": "7d", "generated_at": now, "totals": []any{}, "projects": []any{},
		"now": map[string]any{"measured_at": now.Add(-40 * time.Minute), "seconds": 3.2,
			"totals":   map[string]any{"data_bytes": data, "image_bytes": images, "container_bytes": ctrs, "volume_bytes": vols, "build_cache_bytes": int64(4.6e9)},
			"projects": ps}})
}

func traffic(w http.ResponseWriter, r *http.Request) {
	var hosts []map[string]any
	for _, p := range projects {
		if p.evidence != "checked" {
			continue
		}
		req := int64(len(p.name) * 1371)
		hosts = append(hosts, map[string]any{"host": p.name + ".mobasity.com", "project": p.name, "requests": req,
			"errors_5xx": map[bool]int64{true: 211, false: req / 900}[p.state == "down"], "bytes": req * 18e3, "points": []any{}})
	}
	writeJSON(w, map[string]any{"range": r.URL.Query().Get("range"), "bucket_seconds": 3600, "generated_at": time.Now().UTC(), "hosts": hosts})
}

func apps(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	var days []map[string]any
	var total int64
	for i := 29; i >= 0; i-- {
		n := int64(20 + (i*37)%45)
		total += n
		days = append(days, map[string]any{"date": now.AddDate(0, 0, -i).Format("2006-01-02"), "total": n})
	}
	writeJSON(w, map[string]any{"generated_at": now, "days": 30, "projects": []map[string]any{{
		"project":   "plop",
		"reporters": []map[string]any{{"service": "plop", "container": "plop", "read_at": now.Add(-30 * time.Second), "error": nil, "started_at": now.Add(-5 * 24 * time.Hour)}},
		"counters":  []map[string]any{{"name": "pages_served", "total": total, "days": days}},
		"checks":    []map[string]any{{"service": "plop", "name": "storage_writable", "ok": true, "message": "ok", "since": now.Add(-5 * 24 * time.Hour)}},
		"changes":   []any{},
	}}})
}

// errorCounts is Blackbox's /v1/stats/errors.
func errorCounts(w http.ResponseWriter, r *http.Request) {
	from, _ := time.Parse(time.RFC3339, r.URL.Query().Get("from"))
	to, _ := time.Parse(time.RFC3339, r.URL.Query().Get("to"))
	step := time.Hour
	if r.URL.Query().Get("bucket") == "day" {
		step = 24 * time.Hour
	}
	var buckets []map[string]any
	totals := map[string][2]int64{}
	for t := from.Truncate(step); t.Before(to); t = t.Add(step) {
		h := t.Unix() / int64(step.Seconds())
		for i, p := range []string{"dibs", "lighthouse", "lys", "plop"} {
			e := (h*int64(7+i*3) + int64(i)) % int64(5+i*2)
			if p == "dibs" && to.Sub(t) < 2*time.Hour {
				e += 18
			}
			wn := (h*int64(3+i) + 1) % 4
			if e+wn == 0 {
				continue
			}
			if step == 24*time.Hour {
				e, wn = e*9, wn*11
			}
			buckets = append(buckets, map[string]any{"start": t, "project": p, "error": e, "warn": wn})
			tt := totals[p]
			totals[p] = [2]int64{tt[0] + e, tt[1] + wn}
		}
	}
	var tl []map[string]any
	for _, p := range []string{"dibs", "lighthouse", "lys", "plop"} {
		if t, ok := totals[p]; ok {
			tl = append(tl, map[string]any{"project": p, "error": t[0], "warn": t[1]})
		}
	}
	writeJSON(w, map[string]any{"from": from, "to": to, "bucket": map[bool]string{true: "day", false: "hour"}[step > time.Hour], "buckets": buckets, "totals": tl})
}

func errorGroups(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	g := func(fp, project, service string, count int64, first, last time.Duration, id int64) map[string]any {
		return map[string]any{"fingerprint": fp, "project": project, "service": service, "level": "error", "count": count,
			"first_seen": now.Add(-first), "last_seen": now.Add(-last), "newest_id": id, "newest_message": fp}
	}
	writeJSON(w, map[string]any{"from": now.Add(-24 * time.Hour), "to": now, "level": "error", "total_groups": 4, "groups": []map[string]any{
		g("database: connection refused: is sparkdb running?", "dibs", "dibs", 41, 3*time.Hour, 2*time.Minute, 1200),
		g("Twitch: send failed after <n> retries", "lys", "lys", 17, 20*time.Hour, 25*time.Minute, 1100),
		g("deploy failed at test: go test failed", "lighthouse", "lighthouse", 3, 9*time.Hour, 6*time.Hour, 900),
		g("storage: write <path>: no space left on device", "plop", "plop", 2, 15*time.Hour, 14*time.Hour, 800),
	}})
}
