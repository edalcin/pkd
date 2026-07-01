# Ponytail Audit — pkd

> Generated: 2026-07-01. One-shot report; applies nothing.

## Findings (ranked: biggest cut first)

```
delete: sharp devDependency — unused, never imported in src/ or vite.config.js.
        Replacement: nothing. [frontend/package.json]

yagni:  kin-openapi pulled in for contract tests, but validateResponse() only
        checks HTTP status codes — doc is never used for schema validation.
        Replace loader + Validate with gopkg.in/yaml.v3 (already transitive).
        Kills 6 transitive deps (oasdiff/yaml*, go-openapi/*, mailru/easyjson,
        josharian/intern, mohae/deepcopy, perimeterx/marshmallow).
        [tests/contract/openapi_test.go]

yagni:  ExportNewThrottle / ExportReset / ExportThrottle live in production
        binaries but exist solely for tests. Move to
        internal/server/export_test.go. -3 exported symbols from prod code.
        [internal/server/middleware_throttle.go, server.go]

stdlib: ConstantTimeEqual hand-rolls XOR comparison — comment claims it calls
        subtle.ConstantTimeCompare internally, implementation doesn't.
        Replace body: return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
        [internal/security/csrf.go]

stdlib: copyFile manual 32 KB read loop — io.Copy already buffers at that
        size. Replace body with:
            io.Copy(dst, src); out.Sync()
        [internal/store/backup.go:71-95]

stdlib: trimSpaces custom space/tab stripper duplicates strings.TrimSpace
        (which also handles \r \n \v \f and Unicode space). Drop trimSpaces,
        call strings.TrimSpace directly.
        [internal/server/middleware_throttle.go:134-143]

shrink: errS3NotConfigured is a zero-field struct implementing error for a
        static message. Replace with errors.New(...) at declaration site.
        [internal/server/server.go:85-87]

shrink: NewCSRFToken() is a single-line alias for NewToken(32). Only one
        caller (ensureCSRFCookie). Inline it or keep as named constant; the
        wrapper adds no type safety.
        [internal/security/csrf.go:5-8]

shrink: createAutoSaveInterval() in settings.js is 16 lines to wrap one
        writable with localStorage. The 4-line custom-store pattern is enough:
            const { subscribe, set } = writable(stored ?? 5000)
            export const autoSaveInterval = { subscribe, set(v) { localStorage.setItem(KEY,v); set(v) } }
        [frontend/src/lib/stores/settings.js]
```

## Net

**−~55 lines, −8 deps possible** (1 direct `sharp` + 7 via kin-openapi if tests restructured)
