# Feature Specification: Document Archiving

**Feature Branch**: `005-document-archiving`  
**Created**: 2026-05-01  
**Status**: Draft  
**Input**: User description: "Quero implementar uma nova funcionalidade. Documentos podem ser ativos ou arquivados. A visualização default é de documentos e árvore de documentos ativos. Porém, quero poder arquivar um documento e ele ficará visivel apenas na árvore de documentos arquivados. Quero poder alternar a visualização da árvore de documentos, entre documentos ativos e arquivados. Porém, quero também uma opção de ver uma árvore com todos os documentos. Documentos arquivados devem ter uma identificação visual clara, tanto na árvore como no seu conteúdo. Quero poder desarquivar documentos. Quero que a opção de Filtrar... na barra superior continue buscando em todos os documentos (ativos e arquivados), e que mostre o resultado na árvore de Todos os documentos."

## Clarifications

### Session 2026-05-01

- Q: When a parent document is archived but its children are not, what happens to the children in the "Active" tree? → A: Children are hidden from the Active tree while their parent is archived; they appear only in Archived/All views.
- Q: Can archived documents be edited, or are they read-only while archived? → A: Archived documents are read-only; users must unarchive to edit.
- Q: What should happen when a user tries to archive a document that is currently locked for editing? → A: Archive is blocked; user sees an error message requiring the document to be unlocked before archiving.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Archiving a Document (Priority: P1)

A user wants to remove a document from their active workspace without permanently deleting it. They use a contextual action (e.g., right-click menu or action button) on a document in the tree or in the document view, and select "Archive". The document immediately disappears from the active tree and becomes accessible only in the archived view.

**Why this priority**: This is the core action that enables the entire archiving workflow. Without the ability to archive, no other scenario has value.

**Independent Test**: Can be fully tested by archiving a document and verifying it disappears from the active tree, then switching to the archived view and confirming it appears there with a visual indicator.

**Acceptance Scenarios**:

1. **Given** the user is in the "Active" tree view and has at least one document, **When** they choose to archive a document, **Then** the document is immediately removed from the active tree.
2. **Given** a document has just been archived, **When** the user switches to the "Archived" view, **Then** the archived document appears with a clear visual indicator (e.g., muted styling or an archive icon).
3. **Given** a user opens an archived document, **When** the document content is displayed, **Then** a prominent visual banner or badge shows that the document is archived and the editor is in read-only mode (editing disabled).

---

### User Story 2 - Switching Document Tree Views (Priority: P2)

A user wants to navigate between their active, archived, and complete document collections. A toggle control at the top of the document tree allows switching between "Active", "Archived", and "All" views without a full page reload.

**Why this priority**: Users need to access archived documents after archiving them. The tree view toggle is the primary navigation mechanism for the entire feature.

**Independent Test**: Can be fully tested by having at least one active and one archived document, switching between all three view modes, and verifying each shows only the expected documents.

**Acceptance Scenarios**:

1. **Given** the user opens the application, **When** the document tree loads, **Then** the default view is "Active", showing only active documents.
2. **Given** the user selects the "Archived" view, **When** the tree refreshes, **Then** only archived documents are shown.
3. **Given** the user selects the "All" view, **When** the tree refreshes, **Then** both active and archived documents are visible, with archived documents visually differentiated.
4. **Given** the user is in any view mode, **When** they switch to a different mode, **Then** the transition is immediate (no full page reload).

---

### User Story 3 - Unarchiving a Document (Priority: P3)

A user wants to restore a previously archived document back to active status. They navigate to the "Archived" view, find the document, and choose to unarchive (restore) it. The document moves back to the active tree.

**Why this priority**: Archiving is only useful if it is reversible. Users need confidence that archiving is a soft, non-destructive operation.

**Independent Test**: Can be fully tested by archiving a document, then unarchiving it, and verifying the document returns to the active tree and is no longer in the archived tree.

**Acceptance Scenarios**:

1. **Given** a user has an archived document, **When** they choose to unarchive it, **Then** the document is immediately moved back to the active tree.
2. **Given** a user unarchives a document while in the "Archived" view, **When** the tree updates, **Then** the document is removed from the archived tree immediately.
3. **Given** the user switches to the "Active" view after unarchiving, **When** the tree is displayed, **Then** the restored document appears in its correct position in the hierarchy.

---

### User Story 4 - Searching Across All Documents (Priority: P4)

A user wants to find a specific document using the "Filter..." search bar, regardless of whether it is active or archived. The search returns results from both states and automatically switches the tree to the "All" view so the user can see the full context.

**Why this priority**: Search must not be siloed by archive status. Users may not remember if the document they are looking for has been archived.

**Independent Test**: Can be fully tested by archiving a document, searching for its content via the Filter bar, and confirming the archived document appears in the results with its archived status visually indicated.

**Acceptance Scenarios**:

1. **Given** a user has both active and archived documents, **When** they type in the Filter bar, **Then** results include documents from both active and archived states.
2. **Given** a search is performed, **When** results are displayed, **Then** the document tree automatically switches to the "All" view.
3. **Given** a search returns archived documents in results, **When** shown in the tree, **Then** each archived result is visually marked as archived.
4. **Given** the user clears the filter, **When** the tree returns to its normal state, **Then** the tree reverts to the view mode that was active before the search began.

---

### Edge Cases

- When a parent document is archived, its children retain their active status individually but are hidden from the "Active" tree as long as the parent is archived. They become visible again in the "Active" tree only after the parent is unarchived.
- How does the "Active" tree behave if all documents have been archived (empty state)?
- What happens when a user is currently reading an archived document and unarchives it — does the visual banner disappear immediately?
- Archiving a locked document is blocked: the system displays an error message instructing the user to unlock the document before archiving.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow users to archive any active document via a contextual action in the document tree and in the document content view.
- **FR-002**: System MUST allow users to unarchive any archived document via a contextual action in the document tree and in the document content view.
- **FR-003**: System MUST persist the archived/active status of each document durably across sessions.
- **FR-004**: The document tree MUST support three view modes: "Active" (default on load), "Archived", and "All".
- **FR-005**: The "Active" view MUST display only active (non-archived) documents.
- **FR-006**: The "Archived" view MUST display only archived documents.
- **FR-007**: The "All" view MUST display both active and archived documents together, with archived documents visually differentiated from active ones.
- **FR-008**: Archived documents MUST be visually distinguished in the document tree (e.g., muted color, archive icon, or label).
- **FR-009**: When an archived document is opened, its content view MUST display a prominent visual indicator (e.g., a banner or badge) showing the document is archived.
- **FR-010**: The "Filter..." search bar MUST search across all documents regardless of archive status.
- **FR-011**: When a search is performed via the Filter bar, the document tree MUST switch to the "All" view to display results from both active and archived documents.
- **FR-012**: The view mode toggle (Active / Archived / All) MUST be clearly visible and accessible within the document tree panel.
- **FR-013**: The default view mode on application load MUST always be "Active".
- **FR-014**: When a parent document is archived, its child documents MUST be hidden from the "Active" tree until the parent is unarchived; the children's own archive status is not changed.
- **FR-015**: Archived documents MUST be read-only; the editor and all content-modification actions MUST be disabled while a document is in archived status.
- **FR-016**: Users MUST be able to unarchive a document directly from the document content view (not only from the tree), so they can restore and immediately resume editing.
- **FR-017**: The archive action MUST be blocked if the target document is currently locked; the system MUST display an error message instructing the user to unlock the document first.

### Key Entities

- **Document**: A knowledge base entry that has an archive status (active or archived) and an optional timestamp recording when it was archived.
- **Document Tree**: The hierarchical panel that lists documents, filtered according to the current view mode.
- **View Mode**: The current filter applied to the document tree — one of three values: Active, Archived, or All.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can archive or unarchive any document in under 5 seconds from any view.
- **SC-002**: Switching between Active, Archived, and All tree views completes in under 1 second.
- **SC-003**: Archived documents are immediately distinguishable from active documents in both the tree and content view without any additional user action.
- **SC-004**: The Filter search returns results from both active and archived documents with complete recall — no archived document matching the query is omitted from results.
- **SC-005**: After clearing a search, the tree correctly reverts to the pre-search view mode automatically, without user intervention.

## Assumptions

- Archiving a parent document does NOT automatically archive its child documents; each document's archive status is managed independently. However, children of an archived parent are hidden from the "Active" tree until the parent is unarchived.
- Archived documents are read-only; users can open and read them but cannot edit content. To resume editing, the user must unarchive the document first.
- The archiving action is available to all users who have permission to edit a document; no additional role or permission is required beyond existing write access.
- The view mode selection (Active / Archived / All) resets to "Active" on each new application session load.
- The visual differentiation for archived documents (muted styling, icon) will follow the existing design system and color palette of the application.
- The graph/link visualization is out of scope for this feature; archived documents will remain in the graph without visual differentiation until a future iteration addresses it.
