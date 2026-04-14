# Feature Specification: Personal Knowledge Database (PKD)

**Feature Branch**: `001-personal-knowledge-db` *(directory prefix only — project policy is main-only; no git branch is created)*
**Created**: 2026-04-14
**Status**: Draft
**Input**: User description: Personal Knowledge Database tool inspired by Trilium (https://github.com/TriliumNext/Trilium), but simpler. Hierarchical documents, rich editor with images, hashtag tagging, icons, calendar view, folder/tag filtering, administration menu, universal search, file attachments, public share links, password-protected, PWA, light/dark theme, mobile-friendly. Runs as a Docker image published to `ghcr.io/edalcin/`, with SQLite database and attachments on host-mounted volumes provided via environment variables.

---

## Clarifications

### Session 2026-04-14

- Q: When the PWA is offline, what editing behavior should it support? → A: Read-only offline — the PWA caches already-loaded documents for viewing; editing requires network connectivity and the editor displays a clear "offline — read only" indicator when disconnected.
- Q: How long should deleted documents stay in the trash before being permanently purged? → A: Indefinitely — deleted documents remain in the Trash forever and are only removed when the owner explicitly empties the trash from the Administration area.
- Q: Should the application produce automatic backups on a schedule? → A: No — backups are manual only. The owner is responsible for clicking "Backup now" in the Administration area and/or backing up the host-mounted SQLite file externally. The app never writes automatic snapshots.
- Q: How should the app handle the same user editing the same document from two tabs / devices? → A: Optimistic concurrency via a version token. Each save carries the version the editor loaded; if the server's stored version is newer, the save is rejected and the user is shown a dialog to either overwrite the newer version or discard local changes and reload it. No diff/merge UI.
- Q: What concrete failed-authentication throttling policy should the app enforce? → A: Per source IP, after 5 consecutive failed password attempts the IP is locked out of the authentication endpoint for 30 minutes. The counter resets on a successful login or after the lockout expires.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Private note-taking in a nested hierarchy (Priority: P1)

As the sole owner of the knowledge base, I unlock the app with a master password and capture thoughts as documents arranged in a tree. Any document can contain child documents, so the same structure expresses both "notes" and "folders" — there is no separate folder concept.

**Why this priority**: Without private access and hierarchical CRUD there is no product. Every other feature layers on top of being able to create, nest, rename, move, and delete documents behind a locked door.

**Independent Test**: Launch the app with a configured master password, unlock it, create a root document, nest child documents several levels deep, rename/move/delete them, lock the app, and reopen to confirm everything persists exactly as left. This alone delivers a working single-user notebook.

**Acceptance Scenarios**:

1. **Given** the app is configured with a master password and the user is signed out, **When** the user enters the correct password, **Then** the app unlocks and displays the full document tree.
2. **Given** the user is signed in, **When** the user creates a new document under an existing parent, **Then** the new document appears as a child in the tree and is immediately editable.
3. **Given** a document exists at path A/B/C, **When** the user drags it to a new parent D, **Then** its full subtree moves with it and remains intact.
4. **Given** the user deletes a document that has children, **When** the deletion is confirmed, **Then** the document and its entire subtree are removed (or sent to a recoverable location — see FR-008).
5. **Given** an incorrect password is entered, **When** the user submits it, **Then** access is denied and repeated failures are throttled.
6. **Given** no activity for an extended period, **When** the user returns to the app, **Then** the session is invalidated and re-authentication is required.

---

### User Story 2 - Rich document editing with inline images (Priority: P1)

The user writes documents using a rich editor that supports headings, lists, tables, code blocks, links, text formatting, and inline images that can be pasted, uploaded, and visually resized inside the document.

**Why this priority**: A bare textarea would make the product a worse version of a text file. Rich editing — especially inline, resizable images — is what the user explicitly asked for and is central to a knowledge-capture workflow.

**Independent Test**: Create a document, type and format text across several block types, paste or upload an image, drag its handles to resize it, save, reopen, and verify every element renders identically.

**Acceptance Scenarios**:

1. **Given** the editor is open on a document, **When** the user applies formatting (bold, headings, lists, tables, code, links), **Then** the formatting is saved and re-renders on reload.
2. **Given** the user pastes an image from the clipboard, **When** the paste completes, **Then** the image appears inline and is persisted with the document.
3. **Given** an image is embedded inline, **When** the user drags a corner handle, **Then** the displayed size updates live and persists after saving.
4. **Given** an image file on the user's disk, **When** the user uploads it through the editor, **Then** it is stored and embedded in the document.
5. **Given** unsaved changes exist, **When** the user navigates away, **Then** changes are auto-saved or the user is warned before losing them.

---

### User Story 3 - Hashtag tagging and filtering (Priority: P2)

The user adds hashtags to documents to form a cross-cutting organization layer independent of the tree, then filters the tree or a flat list by one or more tags.

**Why this priority**: Tagging unlocks second-axis organization and must work before search can meaningfully narrow by tag. It is not required for the very first notebook to be useful, but it is the first thing users reach for once they have more than a handful of documents.

**Independent Test**: Tag several documents across different tree locations, open the tag filter, select one tag, and confirm only matching documents are shown regardless of where they live in the tree.

**Acceptance Scenarios**:

1. **Given** a document is open, **When** the user adds a hashtag, **Then** the tag is attached to the document and becomes selectable in the filter list.
2. **Given** multiple documents share a tag across different tree paths, **When** the user filters by that tag, **Then** all matching documents appear regardless of hierarchy.
3. **Given** a tag filter is active, **When** the user selects additional tags, **Then** results narrow to documents matching all selected tags.
4. **Given** the user removes a tag from a document, **When** they save, **Then** the document no longer appears under that tag's filter.

---

### User Story 4 - Universal search across text and tags (Priority: P2)

The user types any substring into a global search box and immediately sees documents whose title, body text, or hashtags contain that substring.

**Why this priority**: Search is the fastest way to recover knowledge and is called out by the user as "super-busca". It depends on documents and tags existing but otherwise stands alone.

**Independent Test**: Create documents containing distinct words and tags, then run substring queries against each and verify matching documents appear with a snippet of the match context.

**Acceptance Scenarios**:

1. **Given** the user types a substring that appears in a document body, **When** the query runs, **Then** that document appears in the results with surrounding context.
2. **Given** the user types text that matches a hashtag, **When** the query runs, **Then** documents carrying that hashtag appear in the results.
3. **Given** the user types text that matches a document title, **When** the query runs, **Then** that document appears near the top of the results.
4. **Given** a search returns no results, **When** the view updates, **Then** the user is shown a clear empty state.

---

### User Story 5 - Chronological calendar browsing (Priority: P3)

The user opens a calendar view and sees documents plotted on the day they were created, so they can browse the knowledge base as a timeline.

**Why this priority**: Useful recall mode but not required for the app to be functional. Adds temporal navigation for users who remember "when" better than "where".

**Independent Test**: Create documents on different dates (or with backdated timestamps), open the calendar, and confirm each document appears on the correct day and opens when clicked.

**Acceptance Scenarios**:

1. **Given** documents exist with varying creation dates, **When** the user opens the calendar view, **Then** each day shows indicators for documents created on it.
2. **Given** a day in the calendar has documents, **When** the user clicks the day, **Then** the list of documents for that day appears and each item opens the document.
3. **Given** the user navigates to a different month, **When** the calendar re-renders, **Then** it shows documents for that month without reloading the whole app.

---

### User Story 6 - File attachments external to the application (Priority: P3)

The user attaches arbitrary files (PDFs, spreadsheets, archives, etc.) to documents. Attachments are stored on a host-mounted path outside the container and survive container rebuilds.

**Why this priority**: Important for a real knowledge base but not part of the minimum viable notebook. Depends on document CRUD being in place.

**Independent Test**: Attach a file to a document, rebuild/replace the container, reopen the document, and confirm the attachment is still listed and downloadable.

**Acceptance Scenarios**:

1. **Given** a document is open, **When** the user uploads a file as an attachment, **Then** the file is saved to the configured external attachments path and listed on the document.
2. **Given** an attachment exists on a document, **When** the user clicks it, **Then** the file downloads or opens.
3. **Given** the attachments volume is missing or unwritable, **When** the user tries to attach a file, **Then** the app shows a clear error and does not corrupt the document.
4. **Given** a document with attachments is deleted, **When** the deletion commits, **Then** its attachment files are also removed from the external path (or flagged for cleanup — see FR-027).

---

### User Story 7 - Public share links (Priority: P3)

The user generates a public URL for a document so that anyone with the link can view it without authenticating. The owner can later revoke the link.

**Why this priority**: Collaborative/publishing feature on top of the core product. Introduces a public surface area that must be carefully scoped because it bypasses the master password.

**Independent Test**: Share a document, open the generated URL in a private browser window (no session), confirm the document renders read-only, then revoke the link and confirm the URL stops working.

**Acceptance Scenarios**:

1. **Given** a document is selected, **When** the user chooses "share via link", **Then** the system generates a URL with an unguessable token and shows it to the user.
2. **Given** a share link exists, **When** an unauthenticated visitor opens the URL, **Then** the document renders in a read-only public view with its text and inline images.
3. **Given** a share link exists, **When** the owner revokes it, **Then** subsequent requests to that URL return a "not available" response.
4. **Given** a shared document is later edited by the owner, **When** a visitor reloads the public URL, **Then** the latest content is shown.
5. **Given** a document contains file attachments, **When** a public visitor opens the share link, **Then** attachments are NOT downloadable through the public view unless the owner explicitly opted in.

---

### User Story 8 - Administration (backup, cleanup, tag maintenance) (Priority: P3)

From an Administration menu, the owner can back up the database, restore from a backup, clean up orphaned data, and bulk-rename or merge hashtags.

**Why this priority**: Operational hygiene — essential for long-term use but not for day-one value.

**Independent Test**: Take a backup, make changes, restore the backup, and confirm the state reverts. Separately, rename a hashtag used by several documents and confirm every affected document now carries the new tag.

**Acceptance Scenarios**:

1. **Given** the admin screen is open, **When** the user triggers a backup, **Then** a downloadable backup file is produced that captures the full database.
2. **Given** a valid backup file, **When** the user uploads it for restore, **Then** the current database is replaced atomically and the app reloads against the restored state.
3. **Given** an old hashtag `#draft` is used by N documents, **When** the user replaces it with `#wip`, **Then** all N documents are updated in one operation.
4. **Given** the user triggers "cleanup", **When** it runs, **Then** orphaned attachment files (those no longer referenced by any document) are removed and the database is compacted.
5. **Given** an admin operation fails mid-way, **When** the user refreshes, **Then** the database remains in a consistent state (either pre- or post-operation, never partial).

---

### User Story 9 - Visual customization: icons, themes, mobile & offline (Priority: P3)

The user assigns icons to documents, toggles between light and dark themes, and uses the app comfortably on a phone, including installing it as a PWA for offline-capable access.

**Why this priority**: Quality-of-life polish. The app is functional without any of these, but users explicitly asked for all three and they materially affect daily use.

**Independent Test**: Pick an icon for a document and verify it appears in the tree; toggle the theme and verify it persists across reloads; open the app on a phone and install it as a PWA; verify the installed PWA opens and displays previously loaded content.

**Acceptance Scenarios**:

1. **Given** a document is selected, **When** the user picks an icon from the icon library, **Then** the icon replaces the default in the tree and all navigation surfaces.
2. **Given** the theme toggle is shown, **When** the user switches between light and dark, **Then** the entire UI updates and the preference is remembered across sessions.
3. **Given** the app is opened on a mobile device, **When** the viewport is narrow, **Then** the layout adapts so navigation and editing remain usable with touch.
4. **Given** the app is served over HTTPS with a valid manifest, **When** the user installs it, **Then** it launches as a standalone PWA and caches assets needed to display already-loaded content when temporarily offline.

---

### Edge Cases

- **Password brute-force**: repeated failed password attempts from the same source must be throttled or temporarily blocked to defend against guessing.
- **Orphaned attachments**: if the external attachments path is unmounted or read-only at runtime, upload and cleanup operations must fail loudly without corrupting documents.
- **Backup during write**: backups taken while a save is in progress must still produce a consistent snapshot (no partial rows or mid-transaction state).
- **Share link collision / prediction**: share tokens must be long and random enough that guessing one is infeasible.
- **Very large images**: pasted or uploaded images above a sensible size limit must be rejected or resized to prevent database bloat.
- **Deeply nested trees**: the UI and navigation must behave predictably at many levels of depth (including on mobile where horizontal space is tight).
- **Circular moves**: a user must not be able to move a document into its own descendant.
- **Tag rename to an existing tag**: replacing `#a` with `#b` when both exist must merge cleanly without creating duplicates on documents already tagged with both.
- **Concurrent edits from two tabs / devices**: saves carry a version token; if the server-stored version is newer than the one the editor loaded, the save is rejected and the user is shown a dialog to either overwrite with their local changes or discard and reload the newer version. No silent loss of work.
- **Public link to a document that later gains a private child**: children are not reachable from the public view; only the exact shared document is public.
- **Restore from backup while share links are active**: links present in the restored backup are re-activated; links created after the backup cease to function.
- **PWA offline mode**: when offline, the editor is locked and shows a clear "offline — read only" indicator. Previously loaded documents remain viewable from the PWA cache, but no new edits, uploads, tag changes, or admin operations are accepted until connectivity returns.

---

## Requirements *(mandatory)*

### Functional Requirements

**Access control & session**

- **FR-001**: The system MUST require the correct master password (supplied to the running application via environment variable at deploy time) before exposing any document, tag, attachment, or administration surface.
- **FR-002**: The system MUST enforce a per-source-IP failed-authentication lockout: after 5 consecutive failed password attempts from the same source IP, that IP MUST be locked out of the authentication endpoint for 30 minutes. The failure counter MUST reset on a successful login or when the lockout period expires. Locked-out requests MUST receive a clear "too many attempts, try again later" response and MUST NOT reveal whether the submitted password was correct.
- **FR-003**: The system MUST expire idle sessions after a configurable period and require re-authentication when they expire.
- **FR-004**: The system MUST never log, display, or transmit the master password in plaintext beyond the initial environment-variable read; the value MUST only be compared via constant-time checks.

**Documents & hierarchy**

- **FR-005**: Users MUST be able to create, rename, move, duplicate, and delete documents in an arbitrarily nested tree where any document may have child documents.
- **FR-006**: The system MUST prevent moving a document into itself or any of its descendants.
- **FR-007**: The system MUST preserve the full subtree when a document is moved, copied, or deleted.
- **FR-008**: Deleting a document MUST move it (and its entire subtree) into a Trash state rather than erasing it immediately. Trashed documents MUST remain recoverable **indefinitely** and MUST only be permanently removed when the owner explicitly runs "empty trash" from the Administration area. Trashed documents MUST NOT appear in the normal tree, search results, tag filters, or calendar, but MUST be visible inside a dedicated Trash view where they can be restored to their original parent (or to the root if the original parent was also trashed).
- **FR-009**: Each document MUST record its creation timestamp and last-modified timestamp.
- **FR-010**: The system MUST auto-save edits frequently enough that unexpected refreshes do not lose more than a few seconds of work.
- **FR-010a**: Each document MUST carry a monotonically increasing version identifier that changes on every successful save. Every save request from the editor MUST include the version the editor loaded; if the stored version is newer, the save MUST be rejected and the client MUST display a conflict dialog offering two choices: (1) overwrite the newer version with the editor's current content, or (2) discard local changes and reload the newer version into the editor. No automatic diff/merge is attempted.

**Rich editor & images**

- **FR-011**: The editor MUST support at minimum: headings, paragraphs, bold/italic/underline, bullet and numbered lists, tables, links, code blocks, and inline images.
- **FR-012**: Users MUST be able to insert images by paste, drag-and-drop, and file upload.
- **FR-013**: Users MUST be able to visually resize inline images using drag handles, and the chosen size MUST persist.
- **FR-014**: The system MUST enforce a maximum accepted image size and reject or transparently downscale images that exceed it.

**Tags**

- **FR-015**: Users MUST be able to attach and remove hashtags on any document, with tag values validated for allowed characters.
- **FR-016**: The system MUST list all tags in use and show, for each, how many documents carry it.
- **FR-017**: Users MUST be able to filter the document view by one or more tags, with multiple tags combined as an AND filter.

**Icons**

- **FR-018**: Users MUST be able to assign an icon to any document, chosen from a library of icons shipped with the app.
- **FR-019**: Assigned icons MUST render wherever the document appears (tree, search results, calendar, breadcrumbs).

**Search**

- **FR-020**: The system MUST provide a single search box that matches user-entered substrings against document titles, document body text, and hashtags.
- **FR-021**: Search results MUST show the matching document title, a snippet of the matching context, and a visual indicator of where the match occurred (title, body, tag).
- **FR-022**: Searches MUST return results for typical queries (single substring over the full knowledge base) within a time budget imperceptible to the user on a standard personal machine.

**Calendar view**

- **FR-023**: The system MUST provide a calendar view that plots documents on their creation date and allows navigation by month.
- **FR-024**: Clicking a day in the calendar MUST open the list of documents created that day and let the user open any of them in the editor.

**Attachments**

- **FR-025**: The system MUST store file attachments on a host-mounted path configured via environment variable at deploy time, never inside the application container's writable layer.
- **FR-026**: The system MUST record, for each attachment, the document it belongs to, its original filename, and its size.
- **FR-027**: When a document is permanently deleted, the system MUST remove its attachment files from the external path (either immediately or via admin cleanup).
- **FR-028**: If the external attachments path is missing, read-only, or full, the system MUST fail attachment uploads with a clear error and MUST NOT corrupt the owning document.

**Share links**

- **FR-029**: Users MUST be able to generate a public share link for any document; the link MUST contain an unguessable, cryptographically random token.
- **FR-030**: Opening a share link MUST render the document in a read-only public view that shows text content and inline images but does NOT expose the tree, tags, attachments, administration, or other documents.
- **FR-031**: The owner MUST be able to revoke a share link at any time; revoked links MUST immediately return a "not available" response.
- **FR-032**: The public share view MUST be clearly marked as public to prevent confusion with the authenticated view.

**Administration**

- **FR-033**: The administration area MUST allow the owner to produce, on demand, a full and consistent backup of the database and download it. Backups are **manual only** — the application MUST NOT schedule or auto-trigger backups. Producing a backup while writes are in flight MUST still yield a consistent snapshot (see SC-004).
- **FR-034**: The administration area MUST allow the owner to restore from a previously produced backup, replacing the current database atomically.
- **FR-035**: The administration area MUST allow the owner to rename a hashtag across all documents that use it, merging cleanly if the target tag already exists.
- **FR-036**: The administration area MUST allow the owner to run a cleanup that removes orphaned attachment files and compacts the database.
- **FR-036a**: The administration area MUST expose an "empty trash" action that permanently deletes every document currently in the Trash (including their attachments) in a single confirmed operation. It MUST also allow permanently deleting individual trashed documents without emptying the whole trash.
- **FR-037**: All destructive administration operations MUST require an explicit confirmation step.

**Presentation, mobile, PWA**

- **FR-038**: The UI MUST offer a light and a dark theme and remember the user's last choice across sessions.
- **FR-039**: The UI MUST remain usable on mobile viewports: navigation, editing, and search MUST all be accessible via touch on a phone-sized screen.
- **FR-040**: The application MUST be installable as a PWA and MUST cache the assets required to display already-loaded content when the network is temporarily unavailable. Offline mode is **read-only**: when the network is unavailable the editor MUST be disabled, MUST NOT accept edits, and MUST display a clearly visible "offline — read only" indicator. No client-side write queue or offline sync engine is in scope.

**Data storage boundary**

- **FR-041**: All persistent data (documents, tags, share tokens, settings) MUST live in a single database file located on a host-mounted path configured via environment variable, so container rebuilds never destroy data.

**Security (cross-cutting)**

- **FR-042**: The system MUST sanitize all rendered document content to prevent cross-site scripting, especially inside the public share view which is reachable without authentication.
- **FR-043**: The system MUST set standard hardening HTTP headers (content security policy, frame protections, MIME sniffing protection, HTTPS enforcement hints) on all responses.
- **FR-044**: File uploads (images, attachments) MUST be validated for type and size, and their original filenames MUST be sanitized before use on disk or in URLs.
- **FR-045**: The system MUST protect against common web vulnerabilities, specifically including but not limited to: injection into queries, path traversal through filenames, cross-site request forgery on state-changing actions, and open redirects from share URLs.

### Key Entities

- **Document**: A node in the knowledge tree. Has a title, rich content (text + inline images), creation and modification timestamps, a monotonic version identifier used for optimistic-concurrency conflict detection on save, a parent document (nullable for roots), an ordered list of children, an optional icon, a set of hashtags, a set of attachments, optionally one or more active share tokens, and a trashed/active status (trashed documents remain indefinitely until the owner empties the trash).
- **Hashtag**: A normalized tag string that can be attached to many documents and used for filtering and search. Participates in rename/merge operations.
- **Attachment**: A file belonging to a document, stored on the external host path. Has an original filename, a stored filename, a size, a MIME type, and a back-reference to its owning document.
- **Share Token**: A cryptographically random opaque identifier that grants public read access to exactly one document. Has a creation time, a status (active / revoked), and a back-reference to the shared document.
- **Session**: A short-lived authenticated context created when the master password is verified; expires on idle timeout.
- **Backup**: A point-in-time, consistent snapshot of the entire database, producible and restorable from the administration area.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user can unlock the app, create their first document, write and format content, and save it in under 60 seconds from opening the URL.
- **SC-002**: A user can find any document by substring of its title, body, or tag in under 2 seconds of keystroke feedback on a knowledge base of at least 5,000 documents on a standard personal machine.
- **SC-003**: 95% of edits made in the rich editor are auto-preserved across an unexpected page refresh without data loss.
- **SC-004**: A backup taken at any moment successfully restores to a working, consistent state in 100% of attempts, including when it was taken while another document was being edited.
- **SC-005**: Bulk-renaming a hashtag used by 1,000 documents completes in a single user-triggered operation and updates every document correctly.
- **SC-006**: The app runs on a phone-sized screen and lets a user create, open, edit, and search documents using only touch input.
- **SC-007**: 0 share-link URLs have been successfully guessed or enumerated in penetration testing against the share token space.
- **SC-008**: A source that has triggered the authentication lockout (5 failed attempts from that IP) can submit at most 10 password guesses per hour against the authentication endpoint, making online guessing of a non-trivial password infeasible within years.
- **SC-009**: Rebuilding or replacing the application container zero times loses any document, attachment, or configuration (because both live on host-mounted volumes).
- **SC-010**: A new user can install the app on their own machine, following the provided UNRAID graphical "docker → add" instructions, and reach the unlock screen without opening a terminal.
- **SC-011**: The running application has no Critical or High severity vulnerabilities reported by a standard dependency/container scanner at release time.

## Assumptions

**Product scope**

- Single-user product. The master password identifies "the owner". There is no concept of multiple user accounts, roles, or sharing between authenticated users.
- Share links are fully public: anyone with the URL can read. There is no per-link password. Security of a share link rests on the unguessability of its token plus the owner's ability to revoke it.
- Share links do not expire automatically. They stay valid until the owner revokes them or the owning document is deleted.
- Public share views show document text and inline images only. File attachments are **not** exposed through the public view in v1.
- "Cleanup" in the administration area means: remove orphaned attachment files (files on the external path that no documents reference) and compact the database. It does not auto-delete documents.
- Deleted documents go into a Trash state and remain recoverable **indefinitely** until the owner explicitly purges them from the Administration area. There is no automatic time-based purge.
- Calendar view plots documents by **creation** date (not modification).
- Hashtag filtering within the tree combines selected tags using AND semantics.
- Tags are **not** inherited from parent documents to children; each document carries its own tags explicitly.
- The editor is rich (headings, lists, tables, code, links, inline resizable images), modeled after what the user referenced in Trilium, but the specific choice of editor component is a planning decision, not a spec requirement.

**Deployment & operation**

- The application is delivered as a single container image published to `ghcr.io/edalcin/`.
- The container receives three things at runtime via environment variables: the master password, the host path for the SQLite database file, and the host path for file attachments. Both paths are mounted as volumes from the host so they survive container rebuilds.
- CI/CD publishes a new container image automatically on every change to the main branch.
- The project uses only the `main` git branch. No feature branches are ever created — this is why the `001-personal-knowledge-db/` directory prefix is used purely as a spec folder name and does **not** correspond to a git branch.
- Deployment instructions are tailored to UNRAID's graphical "docker → add" workflow so the target user can install without a terminal.
- No credentials, tokens, or secrets are ever committed to the repository; all examples in the repository use placeholder values.

**Security posture**

- All persistent data (documents, tags, share tokens, backups) is stored in a SQLite database file. The choice of SQLite is a user constraint driven by simplicity and small container footprint.
- The threat model assumes the app is reachable over HTTPS (TLS is provided by the deployment environment or a reverse proxy, not by the app itself).
- The app treats itself as internet-exposed: all hardening (CSP, CSRF, input sanitization, throttled auth, path-traversal defenses) applies even when the user intends to run it only on a home LAN, because share links create a real public surface.

**Out of scope for v1**

- Multi-user accounts, roles, or per-user permissions.
- Collaborative/real-time editing.
- Automatic/scheduled backups. The owner is responsible for triggering manual backups from the Administration area or for backing up the host-mounted SQLite file externally.
- Offline editing / client-side sync engine. The PWA is read-only when offline.
- Automatic time-based purging of trashed documents. Trash is cleared only when the owner explicitly empties it.
- End-to-end encryption of note content (data at rest is protected by host filesystem permissions and by the master password gate, not by encryption of the SQLite file).
- Cloud sync between two running instances.
- Import from Trilium or other note apps.
- Mobile-app-store native builds (the PWA is the mobile story).
