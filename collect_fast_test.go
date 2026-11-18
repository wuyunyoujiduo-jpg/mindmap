package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/ghsync"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/mcp"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/vault"
)

// What 收集 works out of its clips is worked out once, not at every frame
// (docs/审查清单.md, 26 to 32): each test here counts the work, and fails
// when a frame does it again; and tells that what is kept from a frame to
// the next is of the clips as they are now.

// cardSays is what a clip's card says of it, as the wall draws it: its
// name, and its text or the line under its name.
func cardSays(a *app, c *library.Clip) string {
	look := cardOf(c, a.libQuery)
	return look.label + "\n" + look.body
}

// The cards of a wall are worked out once for their clips' words: a frame
// that changes nothing tries no rule of a secret and cuts no text again.
func TestWallWorkedOutOnce(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	addNotes(t, a, 30, 1500)
	addLinks(t, a, 30, 20000)
	showWall(a, tt)
	// Every card is of a memo the clip keeps: the same one, frame after
	// frame, while the clip's words are the same.
	before := map[string]*cardLook{}
	for _, c := range a.lib.Clips {
		before[c.ID] = cardOf(c, a.libQuery)
	}
	tried := library.RulesTried()
	for range 5 {
		tt.Frame()
	}
	for _, c := range a.lib.Clips {
		if cardOf(c, a.libQuery) != before[c.ID] {
			t.Fatalf("the card of %q was worked out again", c.Label())
		}
	}
	if n := library.RulesTried() - tried; n != 0 {
		t.Fatalf("the rules of a secret were tried %d times in five frames of a wall that did not change", n)
	}
	// The same with a clip's details open, a page of twenty thousand
	// characters, and with the settings' page of the vault over them: the
	// rules were tried on it some ten times a frame.
	a.libSel = a.lib.Clips[0].ID
	tt.Frame()
	tt.Frame()
	tried = library.RulesTried()
	for range 5 {
		tt.Frame()
	}
	details := library.RulesTried() - tried
	a.openVaultSettings() // which counts the clips that hold a secret kept as written
	tt.Frame()
	tt.Frame()
	tried = library.RulesTried()
	for range 5 {
		tt.Frame()
	}
	if settings := library.RulesTried() - tried; details != 0 || settings != 0 {
		t.Fatalf("the rules were tried %d times in five frames of a clip's details, %d of the vault's settings", details, settings)
	}
	// One clip's words changed: the rules are tried on that clip alone.
	tried = library.RulesTried()
	a.lib.Clips[5].Note = "改了一个字"
	tt.Frame()
	tt.Frame()
	a.settingsOpen = false
	tt.Frame()
	if n := library.RulesTried() - tried; n == 0 || n > 12 {
		t.Fatalf("one clip changed, and the rules were tried %d times", n)
	}
	// Timed too, in case something else reads the texts: five frames of
	// sixty cards took over a second.
	start := time.Now()
	for range 5 {
		tt.Frame()
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("five frames of the wall took %v", took)
	}
}

// A card's height is told from no more of its text than fills it, and is
// the height told from all of it.
func TestCardHeightReadsItsStart(t *testing.T) {
	whole := func(clip *library.Clip, cols int) float32 { // as it was told, the whole text cut into lines
		per := 24
		if cols == 1 {
			per = 44
		}
		lines := func(s string, most int) float32 {
			n := 0
			for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
				n += 1 + len([]rune(l))/per
			}
			return float32(min(n, most))
		}
		if isMemo(clip) {
			return 32 + lines(clip.Text, 7)*24 + 34
		}
		h := float32(32 + 24 + 34)
		if sub := cmpOr(cmpOr(cmpOr(clip.Desc, clip.Summary), cmpOr(clip.Note, clip.Text)), clip.URL); sub != "" {
			h += lines(sub, 3) * 22
		}
		return h
	}
	rng := rand.New(rand.NewSource(5))
	pieces := []string{"", "短", "\n", "\n\n", strings.Repeat("长", 23), strings.Repeat("长", 24), strings.Repeat("字", 44), strings.Repeat("x", 300), " ", "abc"}
	for range 3000 {
		var b strings.Builder
		for range rng.Intn(9) {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		for _, clip := range []*library.Clip{{Kind: library.KindText, Text: b.String()}, {Kind: library.KindLink, Title: "页", Text: b.String()}} {
			for _, cols := range []int{1, 3} {
				if got, want := clipTall(clip, cols), whole(clip, cols); got != want {
					t.Fatalf("%q in %d columns: %v tall, %v by the whole text", b.String(), cols, got, want)
				}
			}
		}
	}
}

// The thoughts that name a clip, and the open map's material, are looked
// for once and again when what they are of changes.
func TestMentionsAndMaterialFollow(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	clip, _ := a.addClipText("被提到的一条", "手动")
	other, _ := a.addClipText("另一条", "手动")
	showWall(a, tt)
	a.libSel = clip.ID
	tt.Frame()
	if tt.HasText("提到它的") {
		t.Fatal("named before it was")
	}
	a.setMemoText(other, "另一条，见 <"+clipScheme+clip.ID+">")
	tt.Frame()
	if !tt.HasText("和 1 条相连") || !tt.HasText("提到它的") {
		t.Fatalf("a thought that names it is not told: %q", tt.Texts())
	}
	a.setMemoText(other, "另一条，不提了")
	tt.Frame()
	if tt.HasText("提到它的") {
		t.Fatal("named still, by a thought that names it no more")
	}
	// The open map's material: a clip linked, then deleted.
	a.libOpen = false
	a.panelTab = tabCanvas
	tt.Frame()
	if tt.HasText("本图资料") {
		t.Fatal("material, before any")
	}
	a.mutate("", func() { a.doc.Root.Links = append(a.doc.Root.Links, clipScheme+clip.ID) })
	tt.Frame()
	if !tt.HasText("挂着 1 条资料") {
		t.Fatalf("a clip linked is not counted: %q", tt.Texts())
	}
	if err := a.lib.Delete(clip.ID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("挂着 1 条资料") || !tt.HasText("1 个链接指向已不存在的资料或主题") {
		t.Fatalf("a clip deleted is still counted: %q", tt.Texts())
	}
}

// What a card shows follows the clip: its words changed, a secret typed
// into them, marked and unmarked, sealed, opened, sealed whole, the vault
// locked and turned off, a clip changed by a sync. A card that showed a
// secret because it held none a moment ago is a leak.
func TestCardFollowsTheClip(t *testing.T) {
	vault.Quick()
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	note, err := a.addClipText("家里的 wifi 在楼下", "手动")
	if err != nil {
		t.Fatal(err)
	}
	link := &library.Clip{Kind: library.KindLink, URL: "https://example.com/a", Title: "接口文档", Text: "一页正文"}
	if err := a.lib.Add(link, a.now()); err != nil {
		t.Fatal(err)
	}
	showWall(a, tt)
	const dots = "••••••••"
	check := func(when string, c *library.Clip, shows, hides string, secret bool) {
		t.Helper()
		tt.Frame()
		c = a.lib.Find(c.ID)
		said := cardSays(a, c)
		// As worked out afresh, by a clip that kept nothing.
		anew := *c
		data, _ := json.Marshal(c)
		anew = library.Clip{}
		_ = json.Unmarshal(data, &anew)
		if fresh := cardSays(a, &anew); said != fresh {
			t.Fatalf("%s: the card says %q, the clip's words %q", when, said, fresh)
		}
		if shows != "" && !strings.Contains(said, shows) {
			t.Fatalf("%s: %q is not on the card: %q", when, shows, said)
		}
		if hides != "" && (strings.Contains(said, hides) || tt.HasText(hides)) {
			t.Fatalf("%s: %q shows: %q", when, hides, said)
		}
		for _, x := range tt.Texts() {
			if hides != "" && strings.Contains(x, hides) {
				t.Fatalf("%s: %q shows in the window: %q", when, hides, x)
			}
		}
		if c.Secret() != secret {
			t.Fatalf("%s: a secret %v", when, c.Secret())
		}
		n := 0
		if secret {
			n = 1
		}
		if c.ID == note.ID && a.secretCount() != n {
			t.Fatalf("%s: %d clips said to hold a secret", when, a.secretCount())
		}
	}
	check("as kept", note, "家里的 wifi 在楼下", dots, false)

	// Edited, with no save between: the card is of the words as they are.
	note.Text = "家里的 wifi 在楼下\npassword: hunter2hunter2"
	check("a secret typed, not kept yet", note, "password: "+dots, "hunter2", true)
	a.saveClip(note)
	check("kept", note, "password: "+dots, "hunter2", true)
	note.Text = "家里的 wifi 在楼下，密码问我"
	a.saveClip(note)
	check("the secret taken out", note, "密码问我", dots, false)

	// A part of it marked a secret, and not any more.
	a.markSecret(a.secretField(note.Text), mark{id: note.ID, field: "text", from: 3, to: 7}, func(s string) { a.setMemoText(note, s) })
	check("marked", note, "家里的 "+dots, "wifi", true)
	f := a.secretField(note.Text)
	a.markSecret(f, mark{id: note.ID, field: "text", from: 4, to: 4, un: true}, func(s string) { a.setMemoText(note, s) })
	check("no secret any more", note, "家里的 wifi 在楼下", dots, false)

	// The line under a link's name: its description, else its text.
	link.Text = "接口用 sk-abcdefghijklmnopqrstuvwx 调"
	check("a key in a page", link, "接口用 "+dots, "sk-abc", true)
	link.Desc = "一份接口文档"
	check("its description first", link, "一份接口文档", "sk-abc", true)
	link.Desc, link.Text = "", "一页正文"
	a.saveClip(link)

	// A second password: sealed, the card is as it was; locked, unlocked.
	note.Text = "家里的 wifi 在楼下\npassword: hunter2hunter2"
	a.saveClip(note)
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		t.Fatal(err)
	}
	if a.vaultSweep() != 1 {
		t.Fatal("not sealed")
	}
	check("sealed", note, "password: "+dots, "mm1", true)
	a.lockVault()
	check("locked", note, "password: "+dots, "hunter2", true)
	if err := a.vault.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	a.showSecrets(note) // shown in its details: on its card, dots still
	a.libSel = note.ID
	check("unlocked, its secrets shown in its details", note, "password: "+dots, "", true)
	if text, write := a.fieldText(note, "text", note.Text); !write || !strings.Contains(text, "hunter2hunter2") {
		t.Fatalf("the details do not show the secret asked for: %q", text)
	}
	a.lockVault()
	a.libSel = ""
	check("locked again", note, "password: "+dots, "hunter2", true)
	if text, write := a.fieldText(note, "text", note.Text); write || strings.Contains(text, "hunter2") {
		t.Fatalf("locked, the details show the secret: %q", text)
	}

	// Sealed whole, and back.
	if err := a.vault.Unlock("correct horse"); err != nil {
		t.Fatal(err)
	}
	a.lockWhole(note, true)
	check("sealed whole", note, dots, "wifi", true)
	a.lockWhole(note, false)
	check("whole no more", note, "家里的 wifi 在楼下", "hunter2", true)

	// Changed by a sync: the same clip, with other words.
	path := filepath.Join(a.lib.Dir, "clips", note.ID+".json")
	synced := *a.lib.Find(note.ID)
	data, _ := json.Marshal(&synced)
	other := library.Clip{}
	_ = json.Unmarshal(data, &other)
	other.Text = "另一台电脑改成了这样"
	data, _ = json.MarshalIndent(&other, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(time.Hour)
	_ = os.Chtimes(path, at, at)
	_ = os.Chtimes(filepath.Dir(path), at, at)
	a.checkLibrary()
	check("changed by a sync", note, "另一台电脑改成了这样", "password", false)

	// The vault turned off: opened, and in dots as what is kept as written.
	link.Text = "接口用 sk-abcdefghijklmnopqrstuvwx 调"
	a.saveClip(link)
	check("a key sealed", link, "接口用 "+dots, "sk-abc", true)
	a.vaultOff()
	check("the vault off", link, "接口用 "+dots, "sk-abc", true)
	if !strings.Contains(a.lib.Find(link.ID).Text, "sk-abcdefghijklmnopqrstuvwx") {
		t.Fatal("not kept as written")
	}
}

// A letter typed in a clip's details is kept after a pause, not at once:
// the clip shows it at once; nothing worked out of all the clips is
// worked out again for it; and it is kept when the typing pauses, when
// the field is left, when the details close, before a sync and as the
// app is left.
func TestTypingIsKeptAfterAPause(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	addNotes(t, a, 40, 300)
	note := a.lib.Clips[0]
	file := func() string {
		data, _ := os.ReadFile(filepath.Join(a.lib.Dir, "clips", note.ID+".json"))
		var c library.Clip
		_ = json.Unmarshal(data, &c)
		return c.Text
	}
	now := testNow
	a.now = func() time.Time { return now }
	showWall(a, tt)
	a.libQuery = "缓存" // a search with words: the index is made
	a.libSel = note.ID
	tt.Frame()
	tt.Frame()
	if err := tt.Click("资料内容"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	index, refs, version, was := a.lib.Search("缓存", nil), a.refIndex().built, a.lib.Version, note.Text
	if len(index) == 0 {
		t.Fatal("nothing found")
	}
	for _, r := range "新写的几个字" {
		tt.Type(string(r))
		tt.Frame()
	}
	typed := note.Text
	if typed == was || !strings.Contains(typed, "新") || !strings.Contains(cardSays(a, note), firstRunes(typed, 20)) {
		t.Fatalf("what was typed does not show: %q", typed)
	}
	if file() != was || a.lib.Version != version || !a.waitsTyped(note.ID) {
		t.Fatal("kept at each letter")
	}
	// The typing pauses: kept, once; the links of the maps are as they
	// were, and the clip is found by its new words.
	now = now.Add(autosaveDelay + time.Millisecond)
	tt.Frame()
	if file() != typed || a.waitsTyped(note.ID) || a.lib.Version != version+1 {
		t.Fatalf("not kept after a pause: %q, version %d for %d", file(), a.lib.Version, version)
	}
	if a.refIndex().built != refs {
		t.Fatal("the links of every map were gone through again for a letter typed")
	}
	if hits := a.lib.Search("新写的几个字", nil); len(hits) == 0 || hits[0].Clip != note || hits[0].Coverage != 1 {
		t.Fatalf("not found by what was typed: %d", len(hits))
	}

	// Each way out keeps the last letters.
	type way struct {
		name string
		out  func()
	}
	leave := []way{
		{"the details closed", func() { a.libSel = ""; tt.Frame() }},
		{"another clip chosen", func() { a.libSel = a.lib.Clips[1].ID; tt.Frame() }},
		{"收集 left", func() { a.openTasks(); tt.Frame() }},
		{"the field left", func() { _ = tt.Click("搜索收集"); tt.Frame(); tt.Frame() }},
		{"the app left", func() { a.flush() }},
		{"a sync", func() {
			// Kept before the sync reads the folder, not after.
			a.cfg.SyncRepo, a.cfg.SyncToken = "me/maps", "t"
			a.syncRun = func(context.Context, string, ghsync.Config, func(string)) (ghsync.Result, error) {
				if file() != note.Text {
					t.Error("a sync read the folder before what was typed was kept")
				}
				return ghsync.Result{}, nil
			}
			a.syncNow()
			a.cfg.SyncRepo = ""
		}},
		{"the folder read", func() {
			at := time.Now().Add(time.Hour)
			_ = os.Chtimes(filepath.Join(a.lib.Dir, "clips"), at, at)
			a.checkLibrary()
		}},
		{"the library replaced", func() { a.openLibrary(); note = a.lib.Find(note.ID) }},
	}
	for _, w := range leave {
		name, out := w.name, w.out
		showWall(a, tt)
		a.libQuery = ""
		note = a.lib.Find(note.ID)
		a.libSel = note.ID
		tt.Frame()
		tt.Frame()
		tt.Advance(time.Second) // a click of its own, not the second of two
		if err := tt.Click("资料内容"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		tt.Frame()
		tt.Type("再")
		tt.Frame()
		typed = note.Text
		if file() == typed || !a.waitsTyped(note.ID) {
			t.Fatalf("%s: kept before it", name)
		}
		out()
		if file() != typed || a.waitsTyped(note.ID) {
			t.Fatalf("%s: the last letter is not kept: file %q, typed %q", name, firstRunes(file(), 30), firstRunes(typed, 30))
		}
	}
}

// With a second password set, a clip that holds what is to be sealed is
// kept at once, as it was: a secret is not left in the open for a pause.
func TestTypingASecretIsSealedAtOnce(t *testing.T) {
	vault.Quick()
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	note, _ := a.addClipText("服务器在机房", "手动")
	if _, err := a.vault.Setup("correct horse", a.now()); err != nil {
		t.Fatal(err)
	}
	showWall(a, tt)
	a.libSel = note.ID
	tt.Frame()
	if err := tt.Click("资料内容"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type(" token=zyxwvut")
	tt.Frame()
	if !a.waitsTyped(note.ID) || vault.Has(note.Text) {
		t.Fatalf("no secret yet, and kept or sealed: %q", note.Text)
	}
	tt.Type("s") // the letter that makes it one
	tt.Frame()
	data, _ := os.ReadFile(filepath.Join(a.lib.Dir, "clips", note.ID+".json"))
	if a.waitsTyped(note.ID) || !vault.Has(note.Text) || strings.Contains(string(data), "zyxwvuts") || !strings.Contains(string(data), "mm1.") {
		t.Fatalf("a secret typed is not sealed at once: %q", note.Text)
	}
	if text, write := a.fieldText(note, "text", note.Text); !write || !strings.Contains(text, "token=zyxwvuts") {
		t.Fatalf("the field does not go on showing what was typed: %q", text)
	}
}

// The map a card is on is told from an index of every map's links that
// is made once for the maps as they are: a frame of the wall looks at no
// map's file, and builds nothing of the maps again.
func TestBacklinksWorkedOutOnce(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	addBenchMaps(t, a, 30)
	note, _ := a.addClipText("挂在导图上的一条", "手动")
	for i := range 20 {
		if _, err := a.addClipText(fmt.Sprintf("第 %d 条随记", i), "手动"); err != nil {
			t.Fatal(err)
		}
	}
	showWall(a, tt)
	refs, clips := a.refIndex().built, a.mapClips()
	state := a.mapsState()
	for range 5 {
		tt.Frame()
	}
	if a.refIndex().built != refs || a.mapsState() != state || &a.mapClips()[0] != &clips[0] {
		t.Fatal("the maps were gone through again with nothing changed")
	}
	if len(a.clipBacklinks(note.ID)) != 0 {
		t.Fatal("linked before it was")
	}
	// The open map edited: its links are the links at once.
	a.mutate("", func() { a.doc.Root.Links = append(a.doc.Root.Links, clipScheme+note.ID) })
	if back := a.clipBacklinks(note.ID); len(back) != 1 || a.refIndex().built == refs {
		t.Fatalf("a link made on the open map is not known: %v", back)
	}
	tt.Frame()
	if !tt.HasText("在 " + mapName(a.path)) {
		t.Fatalf("the card does not say its map: %q", tt.Texts())
	}
	// Another map's file changed and the folder scanned: known too.
	other := a.files[len(a.files)-1].Path
	if other == a.path {
		other = a.files[0].Path
	}
	f, err := model.Load(other)
	if err != nil {
		t.Fatal(err)
	}
	f.Sheets[0].Root.Links = []string{clipScheme + note.ID}
	if err := f.Save(other); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(time.Hour)
	_ = os.Chtimes(other, at, at)
	a.refreshFiles()
	if back := a.clipBacklinks(note.ID); len(back) != 2 {
		t.Fatalf("a link made in another map is not known: %v", back)
	}
	// A scan that finds nothing changed builds nothing again.
	refs = a.refIndex().built
	a.refreshFiles()
	if a.refIndex().built != refs {
		t.Fatal("a scan of the folder with nothing changed had the links made again")
	}
	// A thought's link to a topic is a link of the index; its other words
	// are nothing to it.
	topic := a.doc.Root.Children[0]
	thought, _ := a.addClipText("想到这个 <"+a.wholeNodeLink(a.path, topic.ID)+">", "手动")
	if got := a.refIndex().to["node:"+a.path+"#"+topic.ID]; len(got) != 1 || got[0].clip != thought.ID {
		t.Fatalf("a thought's link is not known: %v", got)
	}
	refs = a.refIndex().built
	note.Text = "改了几个字"
	a.saveClip(note)
	if a.refIndex().built != refs {
		t.Fatal("a clip's words changed, and the links of every map were made again")
	}
	thought.Text = "不再链接了"
	a.saveClip(thought)
	if got := a.refIndex().to["node:"+a.path+"#"+topic.ID]; len(got) != 0 {
		t.Fatalf("a link taken out of a thought is still known: %v", got)
	}
}

// What [[ offers is found once for what is typed and what there is; and
// no map is read once enough topics are found.
func TestLinkOffersWorkedOutOnce(t *testing.T) {
	a, _ := newTestApp(t)
	a.cfg.LibraryManual = true
	addBenchMaps(t, a, 12)
	if _, err := a.addClipText("主题句：先记下来", "手动"); err != nil {
		t.Fatal(err)
	}
	a.index = nil // no map read yet
	first := a.linkOffers("主题", 4)
	if len(first) < 5 {
		t.Fatalf("offers: %v", first)
	}
	if read := len(a.index); read > 2 {
		t.Fatalf("%d maps read for four topics", read)
	}
	if again := a.linkOffers("主题", 4); &again[0] != &first[0] {
		t.Fatal("found again with nothing changed")
	}
	if other := a.linkOffers("主题 1", 4); len(other) == 0 || &other[0] == &first[0] {
		t.Fatal("another part typed, the same offers")
	}
	// A clip kept, a topic named on the open map: offered at once.
	if _, err := a.addClipText("主题之外的一条", "手动"); err != nil {
		t.Fatal(err)
	}
	if got := a.linkOffers("主题之外", 4); len(got) != 1 {
		t.Fatalf("a clip just kept is not offered: %v", got)
	}
	a.mutate("", func() { a.doc.Root.Children[0].Title = "主题独一无二" })
	if got := a.linkOffers("主题独一无二", 4); len(got) != 1 || got[0].name != "主题独一无二" {
		t.Fatalf("a topic just named is not offered: %v", got)
	}
}

// A clip kept outside the app, as by an agent, is taken in by reading its
// file alone: the others are the clips they were, and what was worked out
// of them stays.
func TestOutsideChangeReadsOneFile(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	addNotes(t, a, 30, 300)
	showWall(a, tt)
	lib, held := a.lib, a.lib.Clips[3]
	card := cardOf(held, "")
	a.libSel = held.ID
	agent := &library.Clip{ID: library.NewID(), Kind: library.KindText, Text: "agent 记的 password: hunter2hunter2", Created: testNow.Add(time.Hour), Updated: testNow}
	data, _ := json.MarshalIndent(agent, "", "  ")
	path := filepath.Join(a.lib.Dir, "clips", agent.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(time.Hour)
	_ = os.Chtimes(filepath.Dir(path), at, at)
	a.checkLibrary()
	tt.Frame()
	got := a.lib.Find(agent.ID)
	if a.lib != lib || got == nil || len(a.lib.Clips) != 31 || a.lib.Clips[0] != got {
		t.Fatal("the clip kept outside is not taken in, or the library is another")
	}
	if a.lib.Find(held.ID) != held || cardOf(held, "") != card || a.libSel != held.ID {
		t.Fatal("a clip that did not change was read again")
	}
	if !got.Secret() || a.secretCount() != 1 || !tt.HasText("含密钥") {
		t.Fatal("the secret it came with is not counted")
	}
	// Deleted outside: gone, and its details with it.
	a.libSel = agent.ID
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	at = at.Add(time.Hour)
	_ = os.Chtimes(filepath.Dir(path), at, at)
	a.checkLibrary()
	tt.Frame()
	if a.lib.Find(agent.ID) != nil || len(a.lib.Clips) != 30 || a.secretCount() != 0 {
		t.Fatal("a clip deleted outside is still there")
	}
}

// A first indexing takes what there is to do once, makes the vectors a
// batch at a time and writes their file once in a while, not after each.
func TestFirstIndexingWritesNowAndThen(t *testing.T) {
	a, _ := newTestApp(t)
	a.cfg.LibraryManual = true
	const n = 200
	for i := range n {
		if err := a.lib.Add(&library.Clip{Kind: library.KindText, Text: fmt.Sprintf("第 %d 条，讲点什么。", i)}, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	batches, writes := 0, 0
	var size int64 = -1
	a.embed = func(_ context.Context, _ library.Embedder, texts []string) ([][]float32, error) {
		batches++
		// The file as it is when a batch is made: written since the last?
		if info, err := os.Stat(filepath.Join(a.lib.Dir, "vectors.json")); err == nil && info.Size() != size {
			size = info.Size()
			writes++
		}
		out := make([][]float32, len(texts))
		for i := range out {
			out[i] = []float32{1, 0}
		}
		return out, nil
	}
	a.cfg.EmbedURL, a.cfg.EmbedModel = "http://embed", "m"
	a.embedLibrary()
	if want := (n + embedBatch - 1) / embedBatch; batches != want {
		t.Fatalf("%d batches for %d clips, want %d", batches, n, want)
	}
	if writes > batches/embedWriteEvery+1 || writes == 0 {
		t.Fatalf("the vectors were written %d times in %d batches", writes, batches)
	}
	// All of them are kept in the end, and read again by another library.
	l2, _ := library.Open(a.lib.Dir)
	if left := l2.ToEmbed("m"); len(left) != 0 || len(a.lib.ToEmbed("m")) != 0 {
		t.Fatalf("%d clips with no vector kept", len(left))
	}
	if a.embedTotal != 0 || a.embedBusy {
		t.Fatal("the run is not over")
	}
	// One clip changed: its vector alone is made again.
	batches = 0
	a.lib.Clips[5].Text = "改过了"
	a.saveClip(a.lib.Clips[5])
	a.embedLibrary()
	if batches != 1 {
		t.Fatalf("%d batches for one clip changed", batches)
	}
}

// noteHasMarks tells what the pattern it stands for tells.
func TestNoteHasMarks(t *testing.T) {
	pieces := []string{"**", "*", "- ", "-", "1. ", "12.", ". ", "\n", " ", "字", "abc", "**粗**", "3", "  - ", "***"}
	rng := rand.New(rand.NewSource(3))
	marked := 0
	for range 20000 {
		var b strings.Builder
		for range 1 + rng.Intn(8) {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		s := b.String()
		if got, want := noteHasMarks(s), noteMarked.MatchString(s); got != want {
			t.Fatalf("%q: marks %v, the pattern says %v", s, got, want)
		} else if want {
			marked++
		}
	}
	if marked < 2000 {
		t.Fatalf("too few of the texts tried have marks to tell: %d", marked)
	}
}

// More of a wall shown is of the wall as it was narrowed then.
func TestWallBeginsAgainWhenNarrowed(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	for i := range wallStep + 30 {
		if err := a.lib.Add(&library.Clip{Kind: library.KindText, Text: fmt.Sprintf("第 %d 条", i)}, testNow.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	showWall(a, tt)
	a.wallShown = wallStep * 2
	tt.Frame()
	if a.wallShown != wallStep*2 {
		t.Fatal("more of the wall is not kept from a frame to the next")
	}
	a.libQuery = "条"
	tt.Frame()
	if a.wallShown != 0 {
		t.Fatal("narrowed anew, the wall does not begin at its first cards")
	}
}

// Files given to 收集 are copied away from the window, one after the
// other: each one's card comes as its copy is done, a card of the wait
// says how far they are, and what could not be kept is said at the end.
func TestFilesAreCopiedAside(t *testing.T) {
	a, tt := newTestApp(t)
	a.cfg.LibraryManual = true
	// Work away from the window, done when the test says.
	var waiting []func()
	a.async = func(work func() (any, error), done func(any, error)) {
		waiting = append(waiting, func() {
			v, err := work()
			done(v, err)
		})
	}
	step := func() {
		t.Helper()
		if len(waiting) == 0 {
			t.Fatal("no work waits")
		}
		next := waiting[0]
		waiting = waiting[1:]
		next()
	}
	dir := t.TempDir()
	var files []string
	for i, name := range []string{"一.txt", "二.md", "三.txt"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(fmt.Sprintf("第 %d 个文件的内容", i)), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, p)
	}
	files = append(files, filepath.Join(dir, "没有.txt"), dir) // one that is not there, and a folder
	twin := filepath.Join(dir, "一的副本.txt")
	_ = os.WriteFile(twin, []byte("第 0 个文件的内容"), 0o644)
	showWall(a, tt)
	a.dropClips(append(files, twin))
	tt.Frame()
	if len(a.lib.Clips) != 0 || len(waiting) != 1 {
		t.Fatalf("copied in the window's turn: %d clips, %d works", len(a.lib.Clips), len(waiting))
	}
	card := func() *jobCard {
		for _, j := range a.jobs() {
			if j.key == "add-files" {
				return &j
			}
		}
		return nil
	}
	if j := card(); j == nil || !strings.Contains(j.line, "共 4 个") {
		t.Fatalf("no card of the wait: %+v", a.jobs())
	}
	step()
	tt.Frame()
	if len(a.lib.Clips) != 1 || a.lib.Clips[0].Name != "一.txt" || !tt.HasText("一") {
		t.Fatalf("the first card did not come with its copy: %d clips", len(a.lib.Clips))
	}
	if entries, _ := os.ReadDir(a.lib.FilesDir()); len(entries) != 1 {
		t.Fatalf("%d files in the library's folder", len(entries))
	}
	step()
	step()
	if len(a.lib.Clips) != 3 || a.libErr != "" || card() == nil {
		t.Fatalf("three kept: %d clips, said %q", len(a.lib.Clips), a.libErr)
	}
	step() // the one kept already: shown, not kept twice, its copy dropped
	if len(a.lib.Clips) != 3 || len(waiting) != 0 || card() != nil {
		t.Fatalf("done: %d clips, %d works left", len(a.lib.Clips), len(waiting))
	}
	if entries, _ := os.ReadDir(a.lib.FilesDir()); len(entries) != 3 {
		t.Fatalf("%d files in the library's folder: a copy left behind", len(entries))
	}
	if a.lib.Find(a.libSel) == nil {
		t.Fatal("none of them is shown")
	}
	// One that cannot be read is said, and does not stop the others.
	gone := filepath.Join(dir, "读不了.txt")
	_ = os.WriteFile(gone, []byte("x"), 0o644)
	last := filepath.Join(dir, "四.txt")
	_ = os.WriteFile(last, []byte("第四个"), 0o644)
	a.dropClips([]string{gone, last})
	_ = os.Remove(gone) // gone before its turn
	step()
	step()
	if len(a.lib.Clips) != 4 || !strings.Contains(a.libErr, "读不了.txt") || !strings.Contains(a.libErr, "存下了 1 个") {
		t.Fatalf("one failed of two: %d clips, said %q", len(a.lib.Clips), a.libErr)
	}
}

// An agent's decision taken back comes back into the library as it is
// then: put back into the one it was taken from, it was lost when the
// library had been read again meanwhile.
func TestDecisionTakenBackIntoTheLibraryOfNow(t *testing.T) {
	a, _ := newTestApp(t)
	at := a.now().Add(-time.Hour)
	s := &mcp.Server{Dir: a.dir, Now: func() time.Time { at = at.Add(time.Minute); return at }, Agent: "claude-code"}
	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"create_map", map[string]any{"name": "店铺", "outline": "- 结算"}},
		{"log_decision", map[string]any{"map": "店铺", "title": "幂等键放在订单表", "why": "少一张表"}},
	} {
		raw, _ := json.Marshal(call.args)
		if _, err := s.Call(call.tool, raw); err != nil {
			t.Fatal(err)
		}
	}
	a.refreshFiles()
	a.checkLibrary()
	var dec agentItem
	for _, it := range a.agentItems() {
		if it.e.Kind == model.LogDecision {
			dec = it
		}
	}
	if dec.e.Clip == "" || a.lib.Find(dec.e.Clip) == nil {
		t.Fatalf("no decision kept: %+v", dec.e)
	}
	a.takeBack(dec)
	if a.lib.Find(dec.e.Clip) != nil {
		t.Fatal("the decision's clip stays")
	}
	was := a.lib
	a.openLibrary() // read again, as after a sync
	if a.lib == was {
		t.Fatal("the library is the one it was")
	}
	a.noticeUndo()
	if a.lib.Find(dec.e.Clip) == nil {
		t.Fatal("the decision's clip did not come back into the library shown")
	}
}

// The plugin's files are looked at once in a while, not at every frame of
// the settings' page; put there or removed by the app, at once.
func TestPluginLookedAtNowAndThen(t *testing.T) {
	a, _ := newTestApp(t)
	now := testNow
	a.now = func() time.Time { return now }
	if installed, _ := a.pluginState(); installed {
		t.Fatal("installed, in a home just made")
	}
	if err := os.MkdirAll(filepath.Dir(pluginPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(pluginPath(), []byte("model"), 0o644)
	_ = os.WriteFile(pluginPath()+".json", []byte(`{"version": 7}`), 0o644)
	if installed, _ := a.pluginState(); installed {
		t.Fatal("looked at again within the same moment")
	}
	now = now.Add(pluginLook)
	if installed, m := a.pluginState(); !installed || m == nil || m.Version != 7 {
		t.Fatalf("not seen after a while: %v %+v", installed, m)
	}
	a.removePlugin()
	if installed, m := a.pluginState(); installed || m != nil {
		t.Fatal("removed by the app, and still said to be there")
	}
}
