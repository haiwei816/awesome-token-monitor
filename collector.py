import os
import glob
import json
import sqlite3
import time
from datetime import datetime, timezone

def format_model_name(raw_model):
    if not raw_model or str(raw_model).lower() in ['auto', '<synthetic>', 'default', 'none', '']:
        return 'Auto'
    m = str(raw_model).split('/')[-1] # strip provider prefix
    m = m.split('#')[0] # strip #billing=...
    
    # Tongyi Qianwen (Qoder / Alibaba)
    if 'qfmodel' in m.lower():
        return '千问 3.8 Flash'
    if 'qwen-3.8' in m.lower() or 'qwen' in m.lower() or '千问' in m.lower():
        return '千问 3.8 Flash'

    # Moonshot Kimi
    if 'kimi-k3-1' in m.lower() or 'kimi-k3.1' in m.lower() or 'kimi-k3' in m.lower():
        return 'Kimi K3.1'
    if 'kimi' in m.lower():
        return 'Moonshot Kimi'

    # Antigravity / Gemini
    if 'm318' in m.lower() or 'placeholder' in m.lower():
        return 'Gemini 3.8 Flash'
    if 'gemini-3.8' in m.lower():
        return 'Gemini 3.8 Flash'
    if 'gemini-2.5' in m.lower():
        return 'Gemini 2.5 Flash'
    if 'gemini' in m.lower():
        return 'Gemini 3.8 Flash'

    # Claude
    if 'claude-opus-5-5' in m.lower():
        return 'Claude Opus 5.5'
    if 'claude-opus-5' in m.lower():
        return 'Claude Opus 5'
    if 'claude-sonnet-5' in m.lower():
        return 'Claude Sonnet 5'
    if 'claude-fable-5' in m.lower():
        return 'Claude Fable 5'
    if 'claude' in m.lower():
        return m.replace('-', ' ').title()

    # DeepSeek
    if 'deepseek-v4.1' in m.lower() or 'deepseek-v4-1' in m.lower():
        return 'DeepSeek V4.1 Flash'
    if 'deepseek-v4' in m.lower():
        return 'DeepSeek V4 Flash'
    if 'deepseek' in m.lower():
        return 'DeepSeek Flash'

    # GLM
    if 'glm-5.3' in m.lower():
        return 'GLM 5.3 Flash'
    if 'glm' in m.lower():
        return 'GLM Flash'

    # GPT
    if 'gpt-6.1-sol' in m.lower() or 'gpt-6-1-sol' in m.lower():
        return 'GPT-6.1 Sol'
    if 'gpt-6-sol' in m.lower() or 'gpt-5.6-sol' in m.lower():
        return 'GPT-6 Sol'
    if 'gpt-6-luna' in m.lower() or 'gpt-5.6-luna' in m.lower():
        return 'GPT-6 Luna'
    if 'gpt-5.6-terra' in m.lower():
        return 'GPT-5.6 Terra'
    if 'gpt' in m.lower():
        return m.upper()

    # Codex internal
    if 'codex-auto-review' in m.lower():
        return 'Codex Auto Review'

    # Tencent Hunyuan
    if m == 'hy3':
        return 'Tencent Hunyuan 3'
    if m == 'hy4-preview':
        return 'Tencent Hunyuan 4 Preview'
    if 'hy' in m.lower() or 'hunyuan' in m.lower():
        return 'Tencent Hunyuan'

    # Space Bunny
    if 'space-bunny' in m.lower():
        return 'Space Bunny'

    return m

def extract_date_bucket(ts_str):
    if not ts_str:
        return datetime.now(timezone.utc).strftime("%Y-%m-%d")
    ts = str(ts_str).strip()
    if len(ts) >= 10 and ts[4] == '-' and ts[7] == '-':
        return ts[:10]
    try:
        val = float(ts)
        if val > 1e11: # ms
            val /= 1000.0
        return datetime.fromtimestamp(val, timezone.utc).strftime("%Y-%m-%d")
    except Exception:
        return datetime.now(timezone.utc).strftime("%Y-%m-%d")

def resolve_cindy_engine(agent_kind, raw_model=""):
    k = (agent_kind or "").lower()
    if "claude" in k or k == "cc":
        return "claude"
    if "pi" in k:
        return "pi"
    if "codex" in k:
        return "codex"
    m = (raw_model or "").lower()
    if any(x in m for x in ["claude", "deepseek", "glm"]):
        return "claude"
    return "codex"

def get_engine_role(engine):
    if engine == "claude":
        return "Claude Code Engine"
    elif engine == "codex":
        return "Codex Engine"
    elif engine == "pi":
        return "Pi Coding Agent"
    return "AI Engine"

class UniversalTokenCollector:
    def __init__(self, cache_db_path=None):
        if cache_db_path is None:
            cache_db_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "token_monitor_cache.db")
        self.cache_db_path = cache_db_path

        self.home_dir = os.path.expanduser("~")
        self.antigravity_dir = os.path.join(self.home_dir, ".gemini", "antigravity")
        self.claude_projects_dir = os.path.join(self.home_dir, ".claude", "projects")
        self.cindy_global_dir = os.path.join(self.home_dir, "AppData", "Roaming", "CindyGlobal")
        self.cindy_cn_dir = os.path.join(self.home_dir, "AppData", "Roaming", "Cindy")
        self.workbuddy_db = os.path.join(self.home_dir, ".workbuddy", "workbuddy.db")
        self.workbuddy_projects_dir = os.path.join(self.home_dir, ".workbuddy", "projects")
        self.qoder_projects_dir = os.path.join(self.home_dir, ".qoder", "projects")
        self.codex_sessions_dir = os.path.join(self.home_dir, ".codex", "sessions")
        self.zcode_rollout_dir = os.path.join(self.home_dir, ".zcode", "cli", "rollout")

        self._init_db()

    def _get_cache_conn(self):
        conn = sqlite3.connect(self.cache_db_path)
        conn.row_factory = sqlite3.Row
        return conn

    def _init_db(self):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            c.execute("""
            CREATE TABLE IF NOT EXISTS conversations (
                id TEXT PRIMARY KEY,
                conversation_id TEXT,
                agent_type TEXT,
                model_name TEXT,
                model_display_name TEXT,
                title TEXT,
                preview TEXT,
                step_count INT,
                last_modified TEXT,
                project_id TEXT,
                agent_role TEXT,
                parent_conversation_id TEXT,
                nesting_depth INT,
                status TEXT,
                total_input_tokens INT DEFAULT 0,
                total_output_tokens INT DEFAULT 0,
                total_cache_tokens INT DEFAULT 0,
                total_tokens INT DEFAULT 0,
                step_records_count INT DEFAULT 0,
                date_bucket TEXT,
                first_step_time TEXT,
                last_step_time TEXT
            )
            """)

            c.execute("""
            CREATE TABLE IF NOT EXISTS step_records (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                agent_type TEXT,
                conversation_id TEXT,
                step_index INT,
                model_name TEXT,
                model_display_name TEXT,
                source TEXT,
                step_type TEXT,
                status TEXT,
                created_at TEXT,
                date_bucket TEXT,
                input_tokens INT,
                output_tokens INT,
                cache_read_tokens INT,
                tool_calls_count INT DEFAULT 0,
                tool_names TEXT DEFAULT '',
                thinking_length INT DEFAULT 0,
                UNIQUE(conversation_id, step_index)
            )
            """)

            c.execute("""CREATE INDEX IF NOT EXISTS idx_step_created ON step_records(created_at);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_step_date ON step_records(date_bucket);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_step_conv ON step_records(conversation_id);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_step_agent ON step_records(agent_type);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_conv_cid ON conversations(conversation_id);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_conv_date ON conversations(date_bucket);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_conv_agent ON conversations(agent_type);""")
            c.execute("""CREATE INDEX IF NOT EXISTS idx_conv_model ON conversations(model_display_name);""")

            c.execute("""
            CREATE TABLE IF NOT EXISTS file_scan_state (
                file_path TEXT PRIMARY KEY,
                agent_type TEXT,
                mtime REAL,
                file_size INT,
                scanned_at TEXT
            )
            """)
            conn.commit()

    def sync(self):
        """Perform scan across all 7 independent agent engines for all dates."""
        t0 = time.time()
        counts = {}
        counts['antigravity']  = self._sync_antigravity()
        cindy_cnt = self._sync_cindy_global() + self._sync_cindy_cn()
        counts['codex']        = self._sync_codex() + cindy_cnt
        counts['claude']       = self._sync_claude()
        counts['pi']           = self._sync_pi()
        counts['zcode']        = self._sync_zcode()
        counts['workbuddy']    = self._sync_workbuddy()
        counts['qoder']        = self._sync_qoder()
        dur = time.time() - t0
        return {
            "status": "success",
            "counts": counts,
            "duration_seconds": round(dur, 3)
        }

    # 1. Antigravity Scanner
    def _sync_antigravity(self):
        sum_db = os.path.join(self.antigravity_dir, "conversation_summaries.db")
        brain_dir = os.path.join(self.antigravity_dir, "brain")
        if not os.path.exists(sum_db): return 0

        default_model = "gemini-3.8-flash"
        pbtxt_path = os.path.join(self.antigravity_dir, "antigravity_state.pbtxt")
        if os.path.exists(pbtxt_path):
            try:
                with open(pbtxt_path, "r", encoding="utf-8", errors="ignore") as pf:
                    for pline in pf:
                        if "last_selected_agent_model" in pline:
                            default_model = pline.split(":")[-1].strip()
            except Exception: pass

        sum_conn = sqlite3.connect(f"file:{sum_db}?mode=ro", uri=True)
        sum_c = sum_conn.cursor()
        sum_c.execute("SELECT conversation_id, title, preview, step_count, last_modified_time, project_id, agent_name, parent_conversation_id, nesting_depth, status FROM conversation_summaries")
        summaries = {row[0]: row for row in sum_c.fetchall()}
        sum_conn.close()

        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()
        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'antigravity'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        synced = 0
        for cid, meta in summaries.items():
            t_file = os.path.join(brain_dir, cid, ".system_generated", "logs", "transcript.jsonl")
            if not os.path.exists(t_file): continue
            try:
                st = os.stat(t_file)
            except OSError: continue

            last_m, last_s = scan_states.get(t_file, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            synced += 1
            date_groups = {}

            with open(t_file, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line: continue
                    try:
                        item = json.loads(line)
                        created = item.get("created_at", "")
                        inp = item.get("input_tokens")
                        if inp is not None:
                            d_bucket = extract_date_bucket(created)
                            if d_bucket not in date_groups:
                                date_groups[d_bucket] = []
                            step_idx = item.get("step_index", 0)
                            out = item.get("output_tokens", 0)
                            cache = item.get("cache_read_tokens", 0)

                            tool_calls = item.get("tool_calls", [])
                            tool_count = len(tool_calls) if isinstance(tool_calls, list) else 0
                            tool_names = ""
                            if tool_count and isinstance(tool_calls, list):
                                names = [tc.get("function", {}).get("name") or tc.get("toolAction") or tc.get("name") for tc in tool_calls if isinstance(tc, dict)]
                                tool_names = ",".join([str(n) for n in names if n][:5])

                            date_groups[d_bucket].append((
                                "antigravity", cid, step_idx, default_model, format_model_name(default_model),
                                item.get("source", ""), item.get("type", ""), item.get("status", ""), created, d_bucket,
                                inp, out, cache, tool_count, tool_names, len(item.get("thinking", "") or "")
                            ))
                    except Exception: pass

            title = meta[1] or meta[2] or "Antigravity 对话"
            for d_bucket, steps in date_groups.items():
                c.executemany("""
                INSERT OR REPLACE INTO step_records 
                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, steps)

                c_in = sum(s[10] for s in steps)
                c_out = sum(s[11] for s in steps)
                c_cache = sum(s[12] for s in steps)
                first_t = steps[0][8]
                last_t = steps[-1][8]
                c_id = f"{cid}_{d_bucket}"

                c.execute("""
                INSERT OR REPLACE INTO conversations
                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                 step_records_count, date_bucket, first_step_time, last_step_time)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    c_id, cid, "antigravity", default_model, format_model_name(default_model),
                    title, meta[2] or "", len(steps), last_t or meta[4] or "",
                    meta[5] or "outside-of-project", meta[6] or "", meta[7] or "", meta[8] or 0, meta[9] or "",
                    c_in, c_out, c_cache, c_in + c_cache + c_out, len(steps), d_bucket, first_t, last_t
                ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (t_file, "antigravity", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # Cindy Universal DB Scanner (Global & Domestic)
    def _sync_cindy_db(self, db_dir, agent_type, agent_role, default_project):
        if not os.path.exists(db_dir): return 0
        dbs = glob.glob(os.path.join(db_dir, "cindy-*.db"))
        if not dbs: return 0

        synced = 0
        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()

        for db in dbs:
            if 'lock' in db: continue
            try:
                st = os.stat(db)
            except OSError: continue

            c.execute("SELECT mtime, file_size FROM file_scan_state WHERE file_path = ?", (db,))
            row = c.fetchone()
            if row and row['mtime'] == st.st_mtime and row['file_size'] == st.st_size:
                continue

            try:
                con_c = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
                cur_c = con_c.cursor()

                try:
                    cur_c.execute("SELECT id, title, working_dir, model, status, total_token_usage, created_at, updated_at, agent_kind FROM sessions")
                    s_rows = cur_c.fetchall()
                except Exception:
                    cur_c.execute("SELECT id, title, working_dir, model, status, total_token_usage, created_at, updated_at FROM sessions")
                    s_rows = [r + (None,) for r in cur_c.fetchall()]

                sessions_map = {}
                for r in s_rows:
                    sessions_map[r[0]] = {
                        'id': r[0], 'title': r[1], 'working_dir': r[2], 'model': r[3],
                        'status': r[4], 'total_tokens': r[5], 'created_at': r[6], 'updated_at': r[7],
                        'agent_kind': r[8] if len(r) > 8 else None
                    }

                # Precompute message counts per session and day
                daily_msg_counts = {}
                total_msg_counts = {}
                try:
                    cur_c.execute("""
                    SELECT 
                        session_id, 
                        strftime('%Y-%m-%d', datetime(created_at / 1000, 'unixepoch')) as day, 
                        count(*) 
                    FROM messages 
                    GROUP BY session_id, day
                    """)
                    for r in cur_c.fetchall():
                        daily_msg_counts[(r[0], r[1])] = r[2]

                    cur_c.execute("SELECT session_id, count(*) FROM messages GROUP BY session_id")
                    for r in cur_c.fetchall():
                        total_msg_counts[r[0]] = r[1]
                except Exception as e:
                    pass

                cur_c.execute("SELECT day, model, input_tokens, output_tokens, cache_read_tokens FROM daily_model_usage")
                daily_model_usage = {}
                overall_model_usage = {}
                global_totals = {'in': 0, 'out': 0, 'cache': 0}

                for r in cur_c.fetchall():
                    d = r[0]
                    raw_m = (r[1] or '').split('#')[0]
                    in_v = r[2] or 0
                    out_v = r[3] or 0
                    cache_v = r[4] or 0

                    d_key = (d, raw_m)
                    if d_key not in daily_model_usage:
                        daily_model_usage[d_key] = {'in': 0, 'out': 0, 'cache': 0}
                    daily_model_usage[d_key]['in'] += in_v
                    daily_model_usage[d_key]['out'] += out_v
                    daily_model_usage[d_key]['cache'] += cache_v

                    if raw_m not in overall_model_usage:
                        overall_model_usage[raw_m] = {'in': 0, 'out': 0, 'cache': 0}
                    overall_model_usage[raw_m]['in'] += in_v
                    overall_model_usage[raw_m]['out'] += out_v
                    overall_model_usage[raw_m]['cache'] += cache_v

                    global_totals['in'] += in_v
                    global_totals['out'] += out_v
                    global_totals['cache'] += cache_v

                cur_c.execute("SELECT day, session_id, tokens, updated_at FROM daily_session_usage")
                daily_rows = cur_c.fetchall()

                daily_by_day = {}
                for day, sid, tokens, updated_at_ms in daily_rows:
                    if day not in daily_by_day:
                        daily_by_day[day] = []
                    daily_by_day[day].append((sid, tokens, updated_at_ms))

                covered_sessions = set()
                for day, s_list in daily_by_day.items():
                    cur_c.execute("SELECT model, input_tokens, output_tokens, cache_read_tokens FROM daily_model_usage WHERE day = ?", (day,))
                    models_on_day = cur_c.fetchall()
                    avail_mods = []
                    for m in models_on_day:
                        clean_m = (m[0] or '').split('#')[0]
                        m_in = m[1] or 0
                        m_out = m[2] or 0
                        m_cache = m[3] or 0
                        tot = m_in + m_out + m_cache
                        avail_mods.append({
                            'raw': m[0], 'clean': clean_m,
                            'in': m_in, 'out': m_out, 'cache': m_cache,
                            'tot': tot, 'rem': tot
                        })

                    matched_map = {}
                    for sid, tokens, updated_at_ms in s_list:
                        covered_sessions.add(sid)
                        best_m = None
                        for am in avail_mods:
                            if abs(am['rem'] - tokens) <= max(10, tokens * 0.001):
                                best_m = am
                                break
                        if best_m:
                            best_m['rem'] -= tokens
                            matched_map[sid] = best_m

                    for sid, tokens, updated_at_ms in s_list:
                        if sid in matched_map: continue
                        meta = sessions_map.get(sid, {})
                        s_mod = (meta.get('model') or 'gpt-6.1-sol').split('#')[0]
                        best_m = None
                        for am in avail_mods:
                            if am['rem'] > 0 and am['clean'] == s_mod:
                                best_m = am
                                break
                        if best_m:
                            best_m['rem'] -= tokens
                            matched_map[sid] = best_m
                        else:
                            for am in avail_mods:
                                if am['rem'] > 0:
                                    best_m = am
                                    break
                            matched_map[sid] = best_m

                    for sid, tokens, updated_at_ms in s_list:
                        meta = sessions_map.get(sid, {})
                        am = matched_map.get(sid)
                        raw_model = am['clean'] if am else (meta.get('model') or 'gpt-6.1-sol').split('#')[0]
                        mod_disp = format_model_name(raw_model)

                        m_stats = (am if am else None) or daily_model_usage.get((day, raw_model)) or overall_model_usage.get(raw_model) or (global_totals if (global_totals['in'] + global_totals['out'] + global_totals['cache']) > 0 else None)
                        if m_stats and (m_stats['in'] + m_stats['out'] + m_stats['cache']) > 0:
                            tot_m = m_stats['in'] + m_stats['out'] + m_stats['cache']
                            in_t = int(tokens * (m_stats['in'] / tot_m))
                            out_t = int(tokens * (m_stats['out'] / tot_m))
                            cache_t = max(tokens - in_t - out_t, 0)
                        else:
                            in_t = int(tokens * 0.05)
                            out_t = int(tokens * 0.01)
                            cache_t = max(tokens - in_t - out_t, 0)

                        updated_iso = datetime.fromtimestamp(updated_at_ms / 1000.0, timezone.utc).isoformat() if updated_at_ms else (day + "T12:00:00Z")
                        created_iso = datetime.fromtimestamp(meta['created_at'] / 1000.0, timezone.utc).isoformat() if meta.get('created_at') else updated_iso

                        target_engine = resolve_cindy_engine(meta.get('agent_kind'), raw_model)
                        target_role = get_engine_role(target_engine)

                        day_steps = daily_msg_counts.get((sid, day)) or total_msg_counts.get(sid) or 1
                        step_idx = abs(hash(f"{sid}_{day}_{raw_model}")) % 10000000 + 1
                        c.execute("""
                        INSERT OR REPLACE INTO step_records
                        (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                         input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                        """, (
                            target_engine, sid, step_idx, raw_model, mod_disp,
                            "assistant", "DAILY_USAGE", meta.get('status') or "completed", updated_iso, day,
                            in_t, out_t, cache_t, 0, "", 0
                        ))

                        raw_title = (meta.get('title') or '').replace('\ufffd', '').strip()
                        title = raw_title if raw_title else (f"Claude 任务 #{sid[:8]}" if target_engine == 'claude' else f"Codex 任务 #{sid[:8]}")
                        c_id = f"{sid}_{day}_{raw_model}"

                        c.execute("""
                        INSERT OR REPLACE INTO conversations
                        (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                         project_id, agent_role, parent_conversation_id, nesting_depth, status,
                         total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                         step_records_count, date_bucket, first_step_time, last_step_time)
                        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                        """, (
                            c_id, sid, target_engine, raw_model, mod_disp,
                            title, "", day_steps, updated_iso,
                            meta.get('working_dir') or default_project, target_role, "", 0, meta.get('status') or "completed",
                            in_t, out_t, cache_t, tokens, day_steps, day, created_iso, updated_iso
                        ))
                        synced += 1

                for sid, meta in sessions_map.items():
                    if sid not in covered_sessions and (meta.get('total_tokens') or 0) > 0:
                        sess_tokens = meta['total_tokens']
                        static_raw = (meta.get('model') or 'gpt-6.1-sol').split('#')[0]
                        title = (meta.get('title') or '').replace('\ufffd', '').strip() or f"Cindy 会话 #{sid[:8]}"
                        created_iso = datetime.fromtimestamp(meta['created_at'] / 1000.0, timezone.utc).isoformat() if meta.get('created_at') else ""
                        updated_iso = datetime.fromtimestamp(meta['updated_at'] / 1000.0, timezone.utc).isoformat() if meta.get('updated_at') else created_iso

                        cur_c.execute("""
                        SELECT strftime('%Y-%m-%d', datetime(created_at / 1000, 'unixepoch')) as day, count(*)
                        FROM messages
                        WHERE session_id = ?
                        GROUP BY day
                        """, (sid,))
                        sess_msg_days = cur_c.fetchall()
                        tot_sess_msgs = sum(m[1] for m in sess_msg_days) or 1

                        if not sess_msg_days:
                            sess_msg_days = [(extract_date_bucket(meta.get('created_at')), 1)]

                        day_model_entries = []
                        total_day_tokens_found = 0

                        for d, d_msgs in sess_msg_days:
                            cur_c.execute("SELECT model, input_tokens, output_tokens, cache_read_tokens FROM daily_model_usage WHERE day = ?", (d,))
                            d_models = cur_c.fetchall()
                            if d_models:
                                for m_row in d_models:
                                    m_raw = (m_row[0] or '').split('#')[0]
                                    m_in = m_row[1] or 0
                                    m_out = m_row[2] or 0
                                    m_cache = m_row[3] or 0
                                    m_tot = m_in + m_out + m_cache
                                    if m_tot > 0:
                                        day_model_entries.append((d, m_raw, m_in, m_out, m_cache, m_tot, d_msgs))
                                        total_day_tokens_found += m_tot

                        # If models found on active days closely correlate with session total:
                        if day_model_entries and abs(total_day_tokens_found - sess_tokens) < max(sess_tokens * 0.15, 6000000):
                            scale = sess_tokens / total_day_tokens_found if total_day_tokens_found > 0 else 1.0
                            for d, m_raw, m_in, m_out, m_cache, m_tot, d_msgs in day_model_entries:
                                in_t = int(m_in * scale)
                                out_t = int(m_out * scale)
                                cache_t = int(m_cache * scale)
                                cur_tok = in_t + out_t + cache_t
                                mod_disp = format_model_name(m_raw)
                                c_id = f"{sid}_{d}_{m_raw}"
                                step_idx = abs(hash(f"{sid}_{d}_{m_raw}")) % 10000000 + 1
                                d_iso = d + "T12:00:00Z"
                                step_share = max(int(d_msgs * (cur_tok / max(total_day_tokens_found, 1))), 1)

                                target_engine = resolve_cindy_engine(meta.get('agent_kind'), m_raw)
                                target_role = get_engine_role(target_engine)

                                c.execute("""
                                INSERT OR REPLACE INTO step_records
                                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                                """, (
                                    target_engine, sid, step_idx, m_raw, mod_disp,
                                    "assistant", "SESSION_SUMMARY", meta.get('status') or "completed", d_iso, d,
                                    in_t, out_t, cache_t, 0, "", 0
                                ))

                                raw_title = (meta.get('title') or '').replace('\ufffd', '').strip()
                                title = raw_title if raw_title else (f"Claude 任务 #{sid[:8]}" if target_engine == 'claude' else f"Codex 任务 #{sid[:8]}")

                                c.execute("""
                                INSERT OR REPLACE INTO conversations
                                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                                 step_records_count, date_bucket, first_step_time, last_step_time)
                                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                                """, (
                                    c_id, sid, target_engine, m_raw, mod_disp,
                                    title, "", step_share, d_iso,
                                    meta.get('working_dir') or default_project, target_role, "", 0, meta.get('status') or "completed",
                                    in_t, out_t, cache_t, cur_tok, step_share, d, d_iso, d_iso
                                ))
                                synced += 1
                        else:
                            for d, d_msgs in sess_msg_days:
                                day_share = d_msgs / tot_sess_msgs
                                cur_tok = int(sess_tokens * day_share)
                                if cur_tok <= 0: continue

                                cur_c.execute("SELECT model FROM daily_model_usage WHERE day = ? ORDER BY (input_tokens + output_tokens + cache_read_tokens) DESC LIMIT 1", (d,))
                                row_mod = cur_c.fetchone()
                                raw_model = row_mod[0].split('#')[0] if row_mod else static_raw
                                mod_disp = format_model_name(raw_model)

                                target_engine = resolve_cindy_engine(meta.get('agent_kind'), raw_model)
                                target_role = get_engine_role(target_engine)

                                m_stats = daily_model_usage.get((d, raw_model)) or overall_model_usage.get(raw_model) or (global_totals if (global_totals['in'] + global_totals['out'] + global_totals['cache']) > 0 else None)
                                if m_stats and (m_stats['in'] + m_stats['out'] + m_stats['cache']) > 0:
                                    tot_m = m_stats['in'] + m_stats['out'] + m_stats['cache']
                                    in_t = int(cur_tok * (m_stats['in'] / tot_m))
                                    out_t = int(cur_tok * (m_stats['out'] / tot_m))
                                    cache_t = max(cur_tok - in_t - out_t, 0)
                                else:
                                    in_t = int(cur_tok * 0.05)
                                    out_t = int(cur_tok * 0.01)
                                    cache_t = max(cur_tok - in_t - out_t, 0)

                                d_iso = d + "T12:00:00Z"
                                step_idx = abs(hash(f"{sid}_{d}_{raw_model}")) % 10000000 + 1
                                c_id = f"{sid}_{d}_{raw_model}"

                                c.execute("""
                                INSERT OR REPLACE INTO step_records
                                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                                """, (
                                    target_engine, sid, step_idx, raw_model, mod_disp,
                                    "assistant", "SESSION_SUMMARY", meta.get('status') or "completed", d_iso, d,
                                    in_t, out_t, cache_t, 0, "", 0
                                ))

                                raw_title = (meta.get('title') or '').replace('\ufffd', '').strip()
                                title = raw_title if raw_title else (f"Claude 任务 #{sid[:8]}" if target_engine == 'claude' else f"Codex 任务 #{sid[:8]}")

                                c.execute("""
                                INSERT OR REPLACE INTO conversations
                                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                                 step_records_count, date_bucket, first_step_time, last_step_time)
                                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                                """, (
                                    c_id, sid, target_engine, raw_model, mod_disp,
                                    title, "", d_msgs, d_iso,
                                    meta.get('working_dir') or default_project, target_role, "", 0, meta.get('status') or "completed",
                                    in_t, out_t, cache_t, cur_tok, d_msgs, d, d_iso, d_iso
                                ))
                                synced += 1

                con_c.close()
            except Exception as e:
                print(f"[Cindy Scanner] Error reading {db}: {e}")

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (db, agent_type, st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # 2. Cindy Global Scanner
    # 2. Cindy Global Scanner
    def _sync_cindy_global(self):
        return self._sync_cindy_db(self.cindy_global_dir, "cindy_global", "Cindy 国际版", "Cindy-Global")

    # 3. Cindy Domestic (CN) Scanner
    def _sync_cindy_cn(self):
        return self._sync_cindy_db(self.cindy_cn_dir, "cindy_cn", "Cindy 国内版", "Cindy-CN")

    # 4. Claude Code Scanner
    def _sync_claude(self):
        if not os.path.exists(self.claude_projects_dir):
            return 0

        synced = 0
        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()

        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'claude'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        for cf in glob.glob(os.path.join(self.claude_projects_dir, "*", "*.jsonl")):
            try:
                st = os.stat(cf)
            except OSError:
                continue

            last_m, last_s = scan_states.get(cf, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            cid = os.path.splitext(os.path.basename(cf))[0]
            folder_raw = os.path.basename(os.path.dirname(cf))

            # 去重检测：如果该会话是由 Cindy 的 cc-manager 创建，
            # 其消耗已完整通过 Cindy 数据库统计到 claude 引擎中。
            # 为了防止双重计费 (Double Counting)，排除 Cindy 托管的会话。
            if "cindy" in folder_raw.lower() or "cindy" in cf.lower():
                continue

            try:
                with open(cf, "r", encoding="utf-8", errors="ignore") as f_chk:
                    head_chunk = f_chk.read(25000)
                    if "cindy" in head_chunk.lower() or "sdk-ts" in head_chunk:
                        continue
            except Exception:
                pass

            project_path = folder_raw.replace("--", "/").replace("-", "/") if "--" in folder_raw else folder_raw

            date_groups = {}
            title = None
            first_user_prompt = None

            with open(cf, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        obj = json.loads(line)
                        otype = obj.get("type")
                        ts = obj.get("timestamp") or ""

                        if otype == "ai-title":
                            t = obj.get("title")
                            if t and not title:
                                title = t

                        if otype == "user" and not first_user_prompt:
                            msg = obj.get("message", {})
                            content = msg.get("content")
                            if isinstance(content, str):
                                for l in content.split("\n"):
                                    l = l.strip()
                                    if l and not l.startswith("[UI language]") and not l.startswith("<system") and not l.startswith("==") and not l.startswith("[User]"):
                                        first_user_prompt = l[:60]
                                        break
                            elif isinstance(content, list):
                                for item in content:
                                    if isinstance(item, dict) and item.get("type") == "text":
                                        t = item.get("text", "").strip()
                                        if t and not t.startswith("[UI language]") and not t.startswith("<system") and not t.startswith("[Request interrupted"):
                                            first_user_prompt = t[:60]
                                            break

                        msg = obj.get("message", {})
                        if isinstance(msg, dict):
                            usage = msg.get("usage")
                            if usage and isinstance(usage, dict):
                                in_t = usage.get("input_tokens", 0)
                                out_t = usage.get("output_tokens", 0)
                                cache_r = usage.get("cache_read_input_tokens", 0)
                                if (in_t + out_t + cache_r) > 0:
                                    mod = msg.get("model") or "Claude"
                                    if mod == "<synthetic>":
                                        mod = "Claude"
                                    d_bucket = extract_date_bucket(ts)
                                    if d_bucket not in date_groups:
                                        date_groups[d_bucket] = []
                                    step_idx = len(date_groups[d_bucket]) + 1
                                    date_groups[d_bucket].append((
                                        "claude", cid, step_idx, mod, format_model_name(mod),
                                        "assistant", otype, "DONE", ts, d_bucket,
                                        in_t, out_t, cache_r, 0, "", 0
                                    ))
                    except Exception:
                        pass

            if date_groups:
                synced += 1
                final_title = title or first_user_prompt or f"Claude #{cid[:8]}"
                for d_bucket, steps in date_groups.items():
                    if not steps:
                        continue
                    c.executemany("""
                    INSERT OR REPLACE INTO step_records 
                    (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                     input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, steps)

                    c_in = sum(s[10] for s in steps)
                    c_out = sum(s[11] for s in steps)
                    c_cache = sum(s[12] for s in steps)
                    last_mod = steps[-1][8] or f"{d_bucket}T12:00:00Z"
                    first_mod = steps[0][8] or f"{d_bucket}T12:00:00Z"
                    c_id = f"{cid}_{d_bucket}"

                    distinct_models = list(dict.fromkeys(s[4] for s in steps))
                    mod_display = ", ".join(distinct_models)
                    raw_model = steps[-1][3]

                    c.execute("""
                    INSERT OR REPLACE INTO conversations
                    (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                     project_id, agent_role, parent_conversation_id, nesting_depth, status,
                     total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                     step_records_count, date_bucket, first_step_time, last_step_time)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, (
                        c_id, cid, "claude", raw_model, mod_display,
                        final_title, first_user_prompt or "", len(steps), last_mod,
                        project_path, "Claude Code", "", 0, "completed",
                        c_in, c_out, c_cache, c_in + c_cache + c_out, len(steps), d_bucket, first_mod, last_mod
                    ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (cf, "claude", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # 4.5 Pi Coding Agent Scanner
    def _sync_pi(self):
        pi_paths = [
            os.path.join(self.home_dir, "AppData", "Roaming", "Cindy", "pi-agent-home"),
            os.path.join(self.home_dir, "AppData", "Roaming", "CindyGlobal", "pi-agent-home"),
            os.path.join(self.home_dir, ".pi", "sessions")
        ]
        synced = 0
        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()
        for p in pi_paths:
            if not os.path.exists(p):
                continue
            for jf in glob.glob(os.path.join(p, "**", "*.jsonl"), recursive=True):
                try:
                    st = os.stat(jf)
                except OSError:
                    continue
        cache_conn.commit()
        cache_conn.close()
        return synced

    # 5. Codex Scanner
    def _sync_codex(self):
        if not os.path.exists(self.codex_sessions_dir): return 0

        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()
        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'codex'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        synced = 0
        for cs in glob.glob(os.path.join(self.codex_sessions_dir, "*", "*", "*", "*.jsonl")):
            try:
                st = os.stat(cs)
            except OSError: continue

            last_m, last_s = scan_states.get(cs, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            cid = os.path.splitext(os.path.basename(cs))[0].replace("rollout-", "")
            session_model = "gpt-6.1-sol"
            title = None
            date_groups = {}

            with open(cs, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line: continue
                    try:
                        obj = json.loads(line)
                        t = obj.get("type")
                        pl = obj.get("payload", {})
                        ts = obj.get("timestamp") or ""
                        d_bucket = extract_date_bucket(ts)
                        if d_bucket not in date_groups: date_groups[d_bucket] = []

                        if pl.get("model"): session_model = pl.get("model")

                        if t == "session_meta" and not title:
                            cwd = pl.get("cwd", "")
                            if cwd: title = os.path.basename(cwd)

                        if t == "response_item" and not title:
                            cnt_str = pl.get("content")
                            if isinstance(cnt_str, str) and len(cnt_str.strip()) > 1:
                                title = cnt_str[:40].strip()

                        if t == "event_msg":
                            info = pl.get("info", {})
                            if info.get("model"): session_model = info.get("model")
                            last_u = info.get("last_token_usage")
                            if last_u and isinstance(last_u, dict):
                                inp = last_u.get("input_tokens", 0)
                                out = last_u.get("output_tokens", 0)
                                cache = last_u.get("cached_input_tokens", 0)
                                if inp > 0 or out > 0:
                                    uncached_in = max(inp - cache, 0) if cache > 0 else inp
                                    step_idx = len(date_groups[d_bucket]) + 1
                                    date_groups[d_bucket].append((
                                        "codex", cid, step_idx, session_model, format_model_name(session_model),
                                        "assistant", t, "DONE", ts, d_bucket,
                                        uncached_in, out, cache, 0, "", 0
                                    ))
                    except Exception: pass

            for d_bucket, steps in date_groups.items():
                if not steps: continue
                synced += 1
                c.executemany("""
                INSERT OR REPLACE INTO step_records 
                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, steps)

                c_in = sum(s[10] for s in steps)
                c_out = sum(s[11] for s in steps)
                c_cache = sum(s[12] for s in steps)
                final_title = title or f"Codex 任务 #{cid[:8]}"
                c_id = f"{cid}_{d_bucket}"

                c.execute("""
                INSERT OR REPLACE INTO conversations
                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                 step_records_count, date_bucket, first_step_time, last_step_time)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    c_id, cid, "codex", session_model, format_model_name(session_model),
                    final_title, "", len(steps), steps[-1][8],
                    "Codex Workspace", "Codex", "", 0, "completed",
                    c_in, c_out, c_cache, c_in + c_cache + c_out, len(steps), d_bucket, steps[0][8], steps[-1][8]
                ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (cs, "codex", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # 5. ZCode Scanner
    def _sync_zcode(self):
        if not os.path.exists(self.zcode_rollout_dir):
            return 0

        synced = 0
        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()

        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'zcode'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        for zf in glob.glob(os.path.join(self.zcode_rollout_dir, "model-io-*.jsonl")):
            try:
                st = os.stat(zf)
            except OSError:
                continue

            last_m, last_s = scan_states.get(zf, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            base = os.path.splitext(os.path.basename(zf))[0]
            sid_from_file = base[len("model-io-"):] if base.startswith("model-io-") else base

            is_subagent = "subagent" in sid_from_file
            parent_id = "sess_f9aece1d-db68-43aa-97e3-1cd280d71ad9" if is_subagent else ""
            agent_role = "ZCode Subagent" if is_subagent else "ZCode"

            date_groups = {}
            first_user_prompt = None

            with open(zf, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        obj = json.loads(line)
                        sid = obj.get("sessionId") or sid_from_file
                        ts = obj.get("startedAt") or obj.get("completedAt") or ""
                        d_bucket = extract_date_bucket(ts)

                        m_info = obj.get("model", {})
                        raw_model = m_info.get("modelId") if isinstance(m_info, dict) else str(m_info or "GLM-5.3-Flash")
                        mod_disp = format_model_name(raw_model)

                        resp = obj.get("response", {})
                        usage = resp.get("usage", {})
                        in_t = usage.get("inputTokens", 0)
                        out_t = usage.get("outputTokens", 0)
                        cache_t = usage.get("cacheReadTokens", 0)

                        tool_calls = resp.get("toolCalls", [])
                        tool_count = len(tool_calls) if isinstance(tool_calls, list) else 0
                        tool_names = ""
                        if tool_count and isinstance(tool_calls, list):
                            names = [tc.get("name") or tc.get("toolName") for tc in tool_calls if isinstance(tc, dict)]
                            tool_names = ",".join([str(n) for n in names if n][:5])

                        thinking_len = len(resp.get("reasoningText") or "")

                        if not first_user_prompt:
                            req = obj.get("request", {})
                            for m in req.get("messages", []):
                                if m.get("role") == "user":
                                    c_text = str(m.get("content") or "")
                                    if "</system-reminder>" in c_text:
                                        c_text = c_text.split("</system-reminder>", 1)[1]
                                    c_text = c_text.strip()
                                    if c_text.startswith("http"):
                                        parts = c_text.split(" ", 1)
                                        if len(parts) > 1:
                                            c_text = parts[1].strip()
                                    if c_text:
                                        first_user_prompt = c_text[:60]
                                        break

                        if d_bucket not in date_groups:
                            date_groups[d_bucket] = []

                        step_idx = len(date_groups[d_bucket]) + 1
                        date_groups[d_bucket].append((
                            "zcode", sid, step_idx, raw_model, mod_disp,
                            "assistant", "model_io", "DONE", ts, d_bucket,
                            in_t, out_t, cache_t, tool_count, tool_names, thinking_len
                        ))
                    except Exception:
                        pass

            if date_groups:
                synced += 1
                final_title = first_user_prompt or ("ZCode 简历审查与生成" if is_subagent else "ZCode 会话")
                for d_bucket, steps in date_groups.items():
                    if not steps:
                        continue
                    c.executemany("""
                    INSERT OR REPLACE INTO step_records 
                    (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                     input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, steps)

                    c_in = sum(s[10] for s in steps)
                    c_out = sum(s[11] for s in steps)
                    c_cache = sum(s[12] for s in steps)
                    last_mod = steps[-1][8] or f"{d_bucket}T12:00:00Z"
                    first_mod = steps[0][8] or f"{d_bucket}T12:00:00Z"
                    c_id = f"{sid_from_file}_{d_bucket}"

                    distinct_models = list(dict.fromkeys(s[4] for s in steps))
                    mod_display = ", ".join(distinct_models)
                    raw_model = steps[-1][3]

                    c.execute("""
                    INSERT OR REPLACE INTO conversations
                    (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                     project_id, agent_role, parent_conversation_id, nesting_depth, status,
                     total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                     step_records_count, date_bucket, first_step_time, last_step_time)
                    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, (
                        c_id, sid_from_file, "zcode", raw_model, mod_display,
                        final_title, first_user_prompt or "", len(steps), last_mod,
                        "ZCode-Default", agent_role, parent_id, 1 if is_subagent else 0, "completed",
                        c_in, c_out, c_cache, c_in + c_cache + c_out, len(steps), d_bucket, first_mod, last_mod
                    ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (zf, "zcode", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # 6. WorkBuddy Scanner (reads projects/*.jsonl with full turns)
    def _sync_workbuddy(self):
        if not os.path.exists(self.workbuddy_db): return 0

        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()

        sess_map = {}
        try:
            wb_conn = sqlite3.connect(f"file:{self.workbuddy_db}?mode=ro", uri=True)
            wb_c = wb_conn.cursor()
            wb_c.execute("SELECT id, title, model, status, created_at, updated_at, project_id FROM sessions")
            for r in wb_c.fetchall():
                sess_map[r[0]] = {
                    'title': r[1], 'model': r[2], 'status': r[3],
                    'created_at': r[4], 'updated_at': r[5], 'project_id': r[6]
                }
            wb_conn.close()
        except Exception as e:
            print(f"[WorkBuddy Scanner] DB error: {e}")

        synced = 0
        wb_jsonls = glob.glob(os.path.join(self.workbuddy_projects_dir, "*", "*.jsonl"))
        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'workbuddy'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        covered_sessions = set()
        for wbf in wb_jsonls:
            cid = os.path.splitext(os.path.basename(wbf))[0]
            covered_sessions.add(cid)
            try:
                st = os.stat(wbf)
            except OSError: continue

            last_m, last_s = scan_states.get(wbf, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            meta_info = sess_map.get(cid, {})
            file_model = meta_info.get("model")

            meta_path = wbf.replace(".jsonl", ".meta.json")
            if not file_model and os.path.exists(meta_path):
                try:
                    with open(meta_path, "r", encoding="utf-8") as mp:
                        m_obj = json.load(mp)
                        file_model = m_obj.get("responseModelId") or m_obj.get("model")
                except Exception: pass

            session_model = file_model or "hy3"
            title = meta_info.get("title") or f"WorkBuddy 会话 #{cid[:8]}"
            date_groups = {}

            with open(wbf, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line: continue
                    try:
                        d = json.loads(line)
                        msg = d.get("message", {})
                        usage = msg.get("usage") if isinstance(msg, dict) else d.get("usage")
                        if not usage: continue
                        ts = d.get("timestamp") or (msg.get("timestamp") if isinstance(msg, dict) else None) or ""
                        d_bucket = extract_date_bucket(ts)
                        if d_bucket not in date_groups: date_groups[d_bucket] = []

                        mod = file_model or d.get("model") or (msg.get("model") if isinstance(msg, dict) else None) or session_model
                        in_t = usage.get("input_tokens", 0)
                        out_t = usage.get("output_tokens", 0)
                        cache_t = usage.get("cache_read_input_tokens", 0)

                        if in_t > 0 or out_t > 0:
                            uncached_in = max(in_t - cache_t, 0) if cache_t > 0 else in_t
                            step_idx = len(date_groups[d_bucket]) + 1
                            date_groups[d_bucket].append((
                                "workbuddy", cid, step_idx, mod, format_model_name(mod),
                                "assistant", d.get("type", "step"), "completed", str(ts), d_bucket,
                                uncached_in, out_t, cache_t, 0, "", 0
                            ))
                    except Exception: pass

            for d_bucket, steps in date_groups.items():
                if not steps: continue
                synced += 1
                c.executemany("""
                INSERT OR REPLACE INTO step_records 
                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, steps)

                c_in = sum(s[10] for s in steps)
                c_out = sum(s[11] for s in steps)
                c_cache = sum(s[12] for s in steps)
                c_id = f"{cid}_{d_bucket}"
                mod_name = steps[0][3]
                mod_disp = steps[0][4]

                c.execute("""
                INSERT OR REPLACE INTO conversations
                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                 step_records_count, date_bucket, first_step_time, last_step_time)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    c_id, cid, "workbuddy", mod_name, mod_disp,
                    title, "", len(steps), steps[-1][8],
                    "WorkBuddy", "WorkBuddy", "", 0, "completed",
                    c_in, c_out, c_cache, c_in + c_cache + c_out, len(steps), d_bucket, steps[0][8], steps[-1][8]
                ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (wbf, "workbuddy", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        # Fallback for any sessions without jsonl files
        for sid, meta in sess_map.items():
            if sid not in covered_sessions:
                c.execute("SELECT id FROM conversations WHERE conversation_id = ?", (sid,))
                if c.fetchone():
                    continue

                raw_model = meta.get("model") or "hy3"
                d_bucket = extract_date_bucket(meta.get("updated_at") or meta.get("created_at"))
                c_id = f"{sid}_{d_bucket}"
                used_tokens = 0
                try:
                    wb_conn = sqlite3.connect(f"file:{self.workbuddy_db}?mode=ro", uri=True)
                    wb_c = wb_conn.cursor()
                    wb_c.execute("SELECT used FROM session_usage WHERE id = ?", (sid,))
                    urow = wb_c.fetchone()
                    if urow: used_tokens = urow[0] or 0
                    wb_conn.close()
                except Exception: pass

                c.execute("""
                INSERT OR REPLACE INTO conversations
                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                 step_records_count, date_bucket, first_step_time, last_step_time)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    c_id, sid, "workbuddy", raw_model, format_model_name(raw_model),
                    meta.get("title") or f"WorkBuddy 会话 #{sid[:8]}", "", 1, "",
                    "WorkBuddy", "WorkBuddy", "", 0, meta.get("status") or "completed",
                    used_tokens, 0, 0, used_tokens, 1, d_bucket, "", ""
                ))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # 6. Qoder Scanner (captures qfmodel / 千问 3.8 Flash via context_usage_ratio & generation tokens)
    def _sync_qoder(self):
        if not os.path.exists(self.qoder_projects_dir): return 0

        cache_conn = self._get_cache_conn()
        c = cache_conn.cursor()
        c.execute("SELECT file_path, mtime, file_size FROM file_scan_state WHERE agent_type = 'qoder'")
        scan_states = {row['file_path']: (row['mtime'], row['file_size']) for row in c.fetchall()}

        synced = 0
        for qf in glob.glob(os.path.join(self.qoder_projects_dir, "*", "*.jsonl")):
            try:
                st = os.stat(qf)
            except OSError: continue

            last_m, last_s = scan_states.get(qf, (0, 0))
            if st.st_mtime == last_m and st.st_size == last_s:
                continue

            cid = os.path.splitext(os.path.basename(qf))[0]
            project_dir = os.path.basename(os.path.dirname(qf))

            title = None
            first_user = None
            session_model = "qfmodel"
            date_groups = {}
            prev_ctx = 0

            with open(qf, "r", encoding="utf-8", errors="ignore") as f:
                for line in f:
                    line = line.strip()
                    if not line: continue
                    try:
                        d = json.loads(line)
                        otype = d.get("type")
                        ts = d.get("timestamp") or ""
                        d_bucket = extract_date_bucket(ts)
                        if d_bucket not in date_groups: date_groups[d_bucket] = []

                        if d.get("model"): session_model = d.get("model")

                        if otype == "user" and not first_user:
                            msg = d.get("message", {})
                            if isinstance(msg, dict):
                                cnt_str = msg.get("content")
                                if isinstance(cnt_str, str): first_user = cnt_str[:60].strip()
                                elif isinstance(cnt_str, list) and cnt_str:
                                    first_user = str(cnt_str[0].get("text", ""))[:60].strip()

                        if otype == "assistant":
                            msg = d.get("message", {})
                            if isinstance(msg, dict):
                                raw_mod = msg.get("model") or d.get("model") or session_model
                                if raw_mod in ["auto", "<synthetic>"]: raw_mod = "qfmodel"
                                session_model = raw_mod

                                usage = msg.get("usage", {})
                                in_t = usage.get("input_tokens", 0)
                                out_t = usage.get("output_tokens", 0)
                                cache_r = usage.get("cache_read_input_tokens", 0)
                                ratio = usage.get("context_usage_ratio", 0)
                                credits = usage.get("credits", 0)

                                content = msg.get("content", [])
                                text_len = 0
                                if isinstance(content, list):
                                    text_len = sum(len(c.get("thinking", "")) + len(c.get("text", "")) for c in content if isinstance(c, dict))
                                elif isinstance(content, str):
                                    text_len = len(content)

                                # If qfmodel returns 0 for direct tokens, calculate from context_usage_ratio (200K window) & generation length
                                if in_t == 0 and out_t == 0:
                                    if ratio > 0:
                                        ctx = int(ratio * 200000)
                                        out_t = max(int(text_len * 0.75), 100)
                                        unit_credit = (credits / ctx * 1000000) if ctx > 0 else 0

                                        # Multi-turn prompt caching detection (Tongyi Qianwen prompt cache)
                                        if prev_ctx > 0 and (unit_credit < 6.0 or credits < 0.15):
                                            cache_r = min(prev_ctx, ctx)
                                            in_t = max(ctx - cache_r, 100)
                                        else:
                                            cache_r = 0
                                            in_t = ctx
                                        prev_ctx = ctx
                                    elif text_len > 0:
                                        out_t = int(text_len * 0.75)
                                        in_t = out_t * 4
                                else:
                                    if in_t > 0:
                                        prev_ctx = in_t + cache_r

                                if in_t > 0 or out_t > 0:
                                    step_idx = len(date_groups[d_bucket]) + 1
                                    date_groups[d_bucket].append((
                                        "qoder", cid, step_idx, raw_mod, format_model_name(raw_mod),
                                        "assistant", otype or "step", "DONE", ts, d_bucket,
                                        in_t, out_t, cache_r, 0, "", 0
                                    ))
                    except Exception: pass

            for d_bucket, steps in date_groups.items():
                if not steps: continue
                synced += 1
                c.executemany("""
                INSERT OR REPLACE INTO step_records 
                (agent_type, conversation_id, step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                 input_tokens, output_tokens, cache_read_tokens, tool_calls_count, tool_names, thinking_length)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, steps)

                c_in = sum(s[10] for s in steps)
                c_out = sum(s[11] for s in steps)
                c_cache = sum(s[12] for s in steps)
                final_title = title or first_user or f"Qoder 任务 #{cid[:8]}"
                c_id = f"{cid}_{d_bucket}"
                mod_name = steps[0][3]
                mod_disp = steps[0][4]

                c.execute("""
                INSERT OR REPLACE INTO conversations
                (id, conversation_id, agent_type, model_name, model_display_name, title, preview, step_count, last_modified,
                 project_id, agent_role, parent_conversation_id, nesting_depth, status,
                 total_input_tokens, total_output_tokens, total_cache_tokens, total_tokens,
                 step_records_count, date_bucket, first_step_time, last_step_time)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """, (
                    c_id, cid, "qoder", mod_name, mod_disp,
                    final_title, first_user or "", len(steps), steps[-1][8],
                    project_dir, "Qoder Agent", "", 0, "completed",
                    c_in, c_out, c_cache, c_in + c_out + c_cache, len(steps), d_bucket, steps[0][8], steps[-1][8]
                ))

            c.execute("""
            INSERT OR REPLACE INTO file_scan_state (file_path, agent_type, mtime, file_size, scanned_at)
            VALUES (?, ?, ?, ?, ?)
            """, (qf, "qoder", st.st_mtime, st.st_size, datetime.now(timezone.utc).isoformat()))

        cache_conn.commit()
        cache_conn.close()
        return synced

    # --- Query Analytics APIs ---
    def get_summary(self, agent_filter=None, model_filter=None, date_filter=None):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            params = []
            where_clauses = []

            if agent_filter and agent_filter != "all":
                where_clauses.append("agent_type = ?")
                params.append(agent_filter)
            if model_filter and model_filter != "all":
                where_clauses.append("model_display_name = ?")
                params.append(model_filter)
            if date_filter and date_filter != "all":
                where_clauses.append("date_bucket = ?")
                params.append(date_filter)

            where_sql = ("WHERE " + " AND ".join(where_clauses)) if where_clauses else ""

            c.execute(f"""
            SELECT 
                COALESCE(SUM(total_input_tokens), 0) as total_input,
                COALESCE(SUM(total_output_tokens), 0) as total_output,
                COALESCE(SUM(total_cache_tokens), 0) as total_cache,
                COALESCE(SUM(total_tokens), 0) as total_tokens,
                COUNT(DISTINCT conversation_id) as total_convs,
                COALESCE(SUM(step_records_count), 0) as total_steps
            FROM conversations
            {where_sql}
            """, params)
            overall = dict(c.fetchone())

            # Cache hit ratio
            in_and_cache = overall['total_input'] + overall['total_cache']
            overall['cache_hit_ratio'] = round((overall['total_cache'] / in_and_cache * 100), 1) if in_and_cache > 0 else 0.0

            # Today stats (strict today)
            today_str = datetime.now(timezone.utc).strftime("%Y-%m-%d")
            step_params = [today_str]
            step_where = ["date_bucket = ?"]
            if agent_filter and agent_filter != "all":
                step_where.append("agent_type = ?")
                step_params.append(agent_filter)
            if model_filter and model_filter != "all":
                step_where.append("model_display_name = ?")
                step_params.append(model_filter)
            c.execute(f"""
            SELECT 
                COALESCE(SUM(input_tokens), 0) as today_input,
                COALESCE(SUM(output_tokens), 0) as today_output,
                COALESCE(SUM(cache_read_tokens), 0) as today_cache,
                COUNT(id) as today_steps
            FROM step_records
            WHERE {" AND ".join(step_where)}
            """, step_params)
            today_row = dict(c.fetchone())
            overall.update(today_row)
            overall['today_total'] = overall['today_input'] + overall['today_output']

            # Peak context window
            c.execute(f"""
            SELECT MAX(input_tokens) as max_input FROM step_records 
            {where_sql}
            """, params)
            overall['peak_context_window'] = c.fetchone()['max_input'] or 0
            overall['avg_tokens_per_step'] = round(overall['total_tokens'] / overall['total_steps']) if overall['total_steps'] > 0 else 0

            # Agents Breakdown
            c.execute(f"""
            SELECT 
                agent_type,
                COUNT(DISTINCT conversation_id) as conv_count,
                SUM(total_tokens) as total_tokens,
                SUM(total_input_tokens) as input_tokens,
                SUM(total_output_tokens) as output_tokens,
                SUM(total_cache_tokens) as cache_tokens
            FROM conversations
            {where_sql}
            GROUP BY agent_type
            ORDER BY total_tokens DESC
            """, params)
            overall['agents_breakdown'] = [dict(r) for r in c.fetchall()]

            # Models Breakdown
            c.execute(f"""
            SELECT 
                model_display_name,
                COUNT(DISTINCT conversation_id) as conv_count,
                SUM(total_tokens) as total_tokens,
                SUM(total_input_tokens) as input_tokens,
                SUM(total_output_tokens) as output_tokens,
                SUM(total_cache_tokens) as cache_tokens
            FROM conversations
            {where_sql}
            GROUP BY model_display_name
            ORDER BY total_tokens DESC
            """, params)
            overall['models_breakdown'] = [dict(r) for r in c.fetchall()]

            return overall

    def get_daily_records(self, agent_filter=None, model_filter=None, limit=60):
        """Get summarized records grouped by day across all dates."""
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            params = []
            where_clauses = ["date_bucket != ''"]

            if agent_filter and agent_filter != "all":
                where_clauses.append("agent_type = ?")
                params.append(agent_filter)
            if model_filter and model_filter != "all":
                where_clauses.append("model_display_name = ?")
                params.append(model_filter)

            sql = f"""
            SELECT 
                date_bucket,
                COUNT(DISTINCT conversation_id) as conv_count,
                SUM(total_tokens) as total_tokens,
                SUM(total_input_tokens) as input_tokens,
                SUM(total_output_tokens) as output_tokens,
                SUM(total_cache_tokens) as cache_tokens,
                SUM(step_records_count) as total_steps,
                GROUP_CONCAT(DISTINCT agent_type) as agents,
                GROUP_CONCAT(DISTINCT model_display_name) as models
            FROM conversations
            WHERE {" AND ".join(where_clauses)}
            GROUP BY date_bucket
            ORDER BY date_bucket DESC
            LIMIT ?
            """
            params.append(limit)
            c.execute(sql, params)
            rows = []
            today_str = datetime.now(timezone.utc).strftime("%Y-%m-%d")
            for r in c.fetchall():
                d = dict(r)
                d['is_today'] = (d['date_bucket'] == today_str)
                d['agents_list'] = [a.strip() for a in (d.get('agents') or '').split(',') if a.strip()]
                d['models_list'] = [m.strip() for m in (d.get('models') or '').split(',') if m.strip()]
                rows.append(d)
            return rows

    def get_trends(self, granularity="day", days=30, agent_filter=None, model_filter=None, date_filter=None):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            params = []
            where_clauses = ["created_at != ''"]

            if agent_filter and agent_filter != "all":
                where_clauses.append("agent_type = ?")
                params.append(agent_filter)
            if model_filter and model_filter != "all":
                where_clauses.append("model_display_name = ?")
                params.append(model_filter)
            if date_filter and date_filter != "all":
                where_clauses.append("date_bucket = ?")
                params.append(date_filter)

            if date_filter and date_filter != "all":
                time_expr = "substr(created_at, 1, 13) || ':00'"
                limit = 24
            else:
                time_expr = "substr(created_at, 1, 13) || ':00'" if granularity == "hour" else "date_bucket"
                limit = days * 24 if granularity == "hour" else days

            params.append(limit)

            sql = f"""
            SELECT 
                {time_expr} as time_bucket,
                SUM(input_tokens) as input_tokens,
                SUM(output_tokens) as output_tokens,
                SUM(cache_read_tokens) as cache_tokens,
                SUM(input_tokens + output_tokens) as total_tokens,
                COUNT(id) as steps,
                COUNT(DISTINCT conversation_id) as active_convs
            FROM step_records
            WHERE {" AND ".join(where_clauses)}
            GROUP BY time_bucket
            ORDER BY time_bucket ASC
            LIMIT ?
            """
            c.execute(sql, params)
            return [dict(r) for r in c.fetchall()]

    def get_conversations(self, agent_filter=None, model_filter=None, date_filter=None, sort_by="total_tokens", order="desc", search="", limit=50, offset=0):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            params = []
            where_clauses = []

            if agent_filter and agent_filter != "all":
                where_clauses.append("agent_type = ?")
                params.append(agent_filter)
            if model_filter and model_filter != "all":
                where_clauses.append("model_display_name = ?")
                params.append(model_filter)
            if date_filter and date_filter != "all":
                where_clauses.append("date_bucket = ?")
                params.append(date_filter)
            if search:
                where_clauses.append("(title LIKE ? OR conversation_id LIKE ? OR project_id LIKE ? OR model_display_name LIKE ?)")
                p = f"%{search}%"
                params.extend([p, p, p, p])

            where_sql = ("WHERE " + " AND ".join(where_clauses)) if where_clauses else ""

            valid_sort = {
                "total_tokens": "total_tokens",
                "input_tokens": "total_input_tokens",
                "output_tokens": "total_output_tokens",
                "cache_tokens": "total_cache_tokens",
                "step_count": "step_records_count",
                "last_modified": "last_modified",
                "date": "date_bucket"
            }
            sort_col = valid_sort.get(sort_by, "total_tokens")
            sort_dir = "ASC" if order.lower() == "asc" else "DESC"

            if date_filter and date_filter != "all":
                c.execute(f"SELECT COUNT(*) as count FROM conversations {where_sql}", params)
                total_count = c.fetchone()['count']

                sql = f"""
                SELECT * FROM conversations
                {where_sql}
                ORDER BY {sort_col} {sort_dir}
                LIMIT ? OFFSET ?
                """
                params.extend([limit, offset])
                c.execute(sql, params)
                return {
                    "total": total_count,
                    "limit": limit,
                    "offset": offset,
                    "items": [dict(r) for r in c.fetchall()]
                }
            else:
                c.execute(f"SELECT COUNT(DISTINCT conversation_id) as count FROM conversations {where_sql}", params)
                total_count = c.fetchone()['count']

                sql = f"""
                SELECT 
                    conversation_id,
                    agent_type,
                    model_name,
                    GROUP_CONCAT(DISTINCT model_display_name) as model_display_name,
                    title,
                    preview,
                    SUM(step_count) as step_count,
                    MAX(last_modified) as last_modified,
                    project_id,
                    agent_role,
                    parent_conversation_id,
                    nesting_depth,
                    status,
                    SUM(total_input_tokens) as total_input_tokens,
                    SUM(total_output_tokens) as total_output_tokens,
                    SUM(total_cache_tokens) as total_cache_tokens,
                    SUM(total_tokens) as total_tokens,
                    SUM(step_records_count) as step_records_count,
                    MAX(date_bucket) as date_bucket,
                    MIN(first_step_time) as first_step_time,
                    MAX(last_step_time) as last_step_time
                FROM conversations
                {where_sql}
                GROUP BY conversation_id
                ORDER BY {sort_col} {sort_dir}
                LIMIT ? OFFSET ?
                """
                params.extend([limit, offset])
                c.execute(sql, params)
                return {
                    "total": total_count,
                    "limit": limit,
                    "offset": offset,
                    "items": [dict(r) for r in c.fetchall()]
                }

    def get_conversation_detail(self, conversation_id):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            c.execute("SELECT * FROM conversations WHERE conversation_id = ? ORDER BY last_modified DESC LIMIT 1", (conversation_id,))
            conv = c.fetchone()
            if not conv: return None
            conv = dict(conv)

            c.execute("""
            SELECT 
                SUM(total_input_tokens) as total_input_tokens,
                SUM(total_output_tokens) as total_output_tokens,
                SUM(total_cache_tokens) as total_cache_tokens,
                SUM(total_tokens) as total_tokens,
                SUM(step_count) as step_count,
                GROUP_CONCAT(DISTINCT model_display_name) as all_models
            FROM conversations
            WHERE conversation_id = ?
            """, (conversation_id,))
            agg = c.fetchone()
            if agg:
                if agg['total_input_tokens'] is not None: conv['total_input_tokens'] = agg['total_input_tokens']
                if agg['total_output_tokens'] is not None: conv['total_output_tokens'] = agg['total_output_tokens']
                if agg['total_cache_tokens'] is not None: conv['total_cache_tokens'] = agg['total_cache_tokens']
                if agg['total_tokens'] is not None: conv['total_tokens'] = agg['total_tokens']
                if agg['step_count'] is not None: conv['step_count'] = agg['step_count']
                if agg['all_models']: conv['model_display_name'] = agg['all_models']

            c.execute("""
            SELECT step_index, model_name, model_display_name, source, step_type, status, created_at, date_bucket,
                   input_tokens, output_tokens, cache_read_tokens,
                   tool_calls_count, tool_names, thinking_length
            FROM step_records
            WHERE conversation_id = ?
            ORDER BY created_at ASC, step_index ASC
            """, (conversation_id,))
            conv['steps'] = [dict(r) for r in c.fetchall()]

            c.execute("""
            SELECT conversation_id, title, agent_role, model_display_name, total_tokens, total_input_tokens, total_output_tokens, step_records_count
            FROM conversations
            WHERE parent_conversation_id = ?
            """, (conversation_id,))
            conv['subagents'] = [dict(r) for r in c.fetchall()]

            return conv

    def get_models_list(self):
        with self._get_cache_conn() as conn:
            c = conn.cursor()
            c.execute("""
            SELECT 
                model_display_name,
                COUNT(DISTINCT conversation_id) as conv_count,
                SUM(total_tokens) as total_tokens,
                SUM(total_input_tokens) as input_tokens,
                SUM(total_output_tokens) as output_tokens,
                SUM(total_cache_tokens) as cache_tokens
            FROM conversations
            WHERE model_display_name != ''
            GROUP BY model_display_name
            ORDER BY total_tokens DESC
            """)
            return [dict(r) for r in c.fetchall()]
