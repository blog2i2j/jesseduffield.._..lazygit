package git_commands

import (
	"testing"

	"github.com/go-errors/errors"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/commands/oscommands"
	"github.com/jesseduffield/lazygit/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestWorkingTreeStageFile(t *testing.T) {
	runner := oscommands.NewFakeRunner(t).
		ExpectGitArgs([]string{"add", "--", "test.txt"}, "", nil)

	instance := buildWorkingTreeCommands(commonDeps{runner: runner})

	assert.NoError(t, instance.StageFile("test.txt"))
	runner.CheckForMissingCalls()
}

func TestWorkingTreeStageFiles(t *testing.T) {
	runner := oscommands.NewFakeRunner(t).
		ExpectGitArgs([]string{"add", "--", "test.txt", "test2.txt"}, "", nil)

	instance := buildWorkingTreeCommands(commonDeps{runner: runner})

	assert.NoError(t, instance.StageFiles([]string{"test.txt", "test2.txt"}, nil))
	runner.CheckForMissingCalls()
}

func TestWorkingTreeUnstageFile(t *testing.T) {
	type scenario struct {
		testName string
		reset    bool
		runner   *oscommands.FakeCmdObjRunner
		test     func(error)
	}

	scenarios := []scenario{
		{
			testName: "Remove an untracked file from staging",
			reset:    false,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"rm", "--cached", "--force", "--", "test.txt"}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			testName: "Remove a tracked file from staging",
			reset:    true,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "HEAD", "--", "test.txt"}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})
			s.test(instance.UnStageFile([]string{"test.txt"}, s.reset))
		})
	}
}

// these tests don't cover everything, in part because we already have an integration
// test which does cover everything. I don't want to unnecessarily assert on the 'how'
// when the 'what' is what matters
func TestWorkingTreeDiscardAllFileChanges(t *testing.T) {
	type scenario struct {
		testName      string
		file          *models.File
		removeFile    func(string) error
		runner        *oscommands.FakeCmdObjRunner
		expectedError string
	}

	scenarios := []scenario{
		{
			testName: "An error occurred when resetting",
			file: &models.File{
				Path:             "test",
				HasStagedChanges: true,
			},
			removeFile: func(string) error { return nil },
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "--", "test"}, "", errors.New("error")),
			expectedError: "error",
		},
		{
			testName: "An error occurred when removing file",
			file: &models.File{
				Path:    "test",
				Tracked: false,
				Added:   true,
			},
			removeFile: func(string) error {
				return errors.New("an error occurred when removing file")
			},
			runner:        oscommands.NewFakeRunner(t),
			expectedError: "an error occurred when removing file",
		},
		{
			testName: "An error occurred with checkout",
			file: &models.File{
				Path:             "test",
				Tracked:          true,
				HasStagedChanges: false,
			},
			removeFile: func(string) error { return nil },
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "--", "test"}, "", errors.New("error")),
			expectedError: "error",
		},
		{
			testName: "Checkout only",
			file: &models.File{
				Path:             "test",
				Tracked:          true,
				HasStagedChanges: false,
			},
			removeFile: func(string) error { return nil },
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "--", "test"}, "", nil),
			expectedError: "",
		},
		{
			testName: "Reset and checkout staged changes",
			file: &models.File{
				Path:             "test",
				Tracked:          true,
				HasStagedChanges: true,
			},
			removeFile: func(string) error { return nil },
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "--", "test"}, "", nil).
				ExpectGitArgs([]string{"checkout", "--", "test"}, "", nil),
			expectedError: "",
		},
		{
			testName: "Reset and checkout merge conflicts",
			file: &models.File{
				Path:              "test",
				Tracked:           true,
				HasMergeConflicts: true,
			},
			removeFile: func(string) error { return nil },
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "--", "test"}, "", nil).
				ExpectGitArgs([]string{"checkout", "--", "test"}, "", nil),
			expectedError: "",
		},
		{
			testName: "Reset and remove",
			file: &models.File{
				Path:             "test",
				Tracked:          false,
				Added:            true,
				HasStagedChanges: true,
			},
			removeFile: func(filename string) error {
				assert.Equal(t, "test", filename)
				return nil
			},
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "--", "test"}, "", nil),
			expectedError: "",
		},
		{
			testName: "Remove only",
			file: &models.File{
				Path:             "test",
				Tracked:          false,
				Added:            true,
				HasStagedChanges: false,
			},
			removeFile: func(filename string) error {
				assert.Equal(t, "test", filename)
				return nil
			},
			runner:        oscommands.NewFakeRunner(t),
			expectedError: "",
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner, removeFile: s.removeFile})
			err := instance.DiscardAllFileChanges(s.file)

			if s.expectedError == "" {
				assert.Nil(t, err)
			} else {
				assert.Equal(t, s.expectedError, err.Error())
			}
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeDiff(t *testing.T) {
	type scenario struct {
		testName            string
		file                *models.File
		plain               bool
		cached              bool
		ignoreWhitespace    bool
		contextSize         uint64
		similarityThreshold int
		runner              *oscommands.FakeCmdObjRunner
	}

	const expectedResult = "pretend this is an actual git diff"

	scenarios := []scenario{
		{
			testName: "Default case",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               false,
			cached:              false,
			ignoreWhitespace:    false,
			contextSize:         3,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=always", "--find-renames=50%", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "cached",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               false,
			cached:              true,
			ignoreWhitespace:    false,
			contextSize:         3,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=always", "--find-renames=50%", "--cached", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "plain",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               true,
			cached:              false,
			ignoreWhitespace:    false,
			contextSize:         3,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=never", "--find-renames=50%", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "File not tracked and file has no staged changes",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          false,
			},
			plain:               false,
			cached:              false,
			ignoreWhitespace:    false,
			contextSize:         3,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=always", "--find-renames=50%", "--no-index", "--", "/dev/null", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "Default case (ignore whitespace)",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               false,
			cached:              false,
			ignoreWhitespace:    true,
			contextSize:         3,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=always", "--ignore-all-space", "--find-renames=50%", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "Show diff with custom context size",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               false,
			cached:              false,
			ignoreWhitespace:    false,
			contextSize:         17,
			similarityThreshold: 50,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=17", "--color=always", "--find-renames=50%", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName: "Show diff with custom similarity threshold",
			file: &models.File{
				Path:             "test.txt",
				HasStagedChanges: false,
				Tracked:          true,
			},
			plain:               false,
			cached:              false,
			ignoreWhitespace:    false,
			contextSize:         3,
			similarityThreshold: 33,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--color=always", "--find-renames=33%", "--", "test.txt"}, expectedResult, nil),
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			userConfig := config.GetDefaultConfig()
			userConfig.Git.IgnoreWhitespaceInDiffView = s.ignoreWhitespace
			userConfig.Git.DiffContextSize = s.contextSize
			userConfig.Git.RenameSimilarityThreshold = s.similarityThreshold
			repoPaths := RepoPaths{
				worktreePath: "/path/to/worktree",
			}

			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner, userConfig: userConfig, appState: &config.AppState{}, repoPaths: &repoPaths})
			result := instance.WorktreeFileDiff(s.file, s.plain, s.cached)
			assert.Equal(t, expectedResult, result)
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeShowFileDiff(t *testing.T) {
	type scenario struct {
		testName         string
		from             string
		to               string
		reverse          bool
		plain            bool
		ignoreWhitespace bool
		contextSize      uint64
		runner           *oscommands.FakeCmdObjRunner
	}

	const expectedResult = "pretend this is an actual git diff"

	scenarios := []scenario{
		{
			testName:         "Default case",
			from:             "1234567890",
			to:               "0987654321",
			reverse:          false,
			plain:            false,
			ignoreWhitespace: false,
			contextSize:      3,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "-c", "diff.noprefix=false", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--no-renames", "--color=always", "1234567890", "0987654321", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName:         "Show diff with custom context size",
			from:             "1234567890",
			to:               "0987654321",
			reverse:          false,
			plain:            false,
			ignoreWhitespace: false,
			contextSize:      123,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "-c", "diff.noprefix=false", "diff", "--no-ext-diff", "--submodule", "--unified=123", "--no-renames", "--color=always", "1234567890", "0987654321", "--", "test.txt"}, expectedResult, nil),
		},
		{
			testName:         "Default case (ignore whitespace)",
			from:             "1234567890",
			to:               "0987654321",
			reverse:          false,
			plain:            false,
			ignoreWhitespace: true,
			contextSize:      3,
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"-C", "/path/to/worktree", "-c", "diff.noprefix=false", "diff", "--no-ext-diff", "--submodule", "--unified=3", "--no-renames", "--color=always", "1234567890", "0987654321", "--ignore-all-space", "--", "test.txt"}, expectedResult, nil),
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			userConfig := config.GetDefaultConfig()
			userConfig.Git.IgnoreWhitespaceInDiffView = s.ignoreWhitespace
			userConfig.Git.DiffContextSize = s.contextSize
			repoPaths := RepoPaths{
				worktreePath: "/path/to/worktree",
			}

			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner, userConfig: userConfig, appState: &config.AppState{}, repoPaths: &repoPaths})

			result, err := instance.ShowFileDiff(s.from, s.to, s.reverse, "test.txt", s.plain)
			assert.NoError(t, err)
			assert.Equal(t, expectedResult, result)
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeCheckoutFile(t *testing.T) {
	type scenario struct {
		testName   string
		commitHash string
		fileName   string
		runner     *oscommands.FakeCmdObjRunner
		test       func(error)
	}

	scenarios := []scenario{
		{
			testName:   "typical case",
			commitHash: "11af912",
			fileName:   "test999.txt",
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "11af912", "--", "test999.txt"}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
		{
			testName:   "returns error if there is one",
			commitHash: "11af912",
			fileName:   "test999.txt",
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "11af912", "--", "test999.txt"}, "", errors.New("error")),
			test: func(err error) {
				assert.Error(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})

			s.test(instance.CheckoutFile(s.commitHash, s.fileName))
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeDiscardUnstagedFileChanges(t *testing.T) {
	type scenario struct {
		testName string
		file     *models.File
		runner   *oscommands.FakeCmdObjRunner
		test     func(error)
	}

	scenarios := []scenario{
		{
			testName: "valid case",
			file:     &models.File{Path: "test.txt"},
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "--", "test.txt"}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})
			s.test(instance.DiscardUnstagedFileChanges(s.file))
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeDiscardAnyUnstagedFileChanges(t *testing.T) {
	type scenario struct {
		testName string
		runner   *oscommands.FakeCmdObjRunner
		test     func(error)
	}

	scenarios := []scenario{
		{
			testName: "valid case",
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"checkout", "--", "."}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})
			s.test(instance.DiscardAnyUnstagedFileChanges())
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeRemoveUntrackedFiles(t *testing.T) {
	type scenario struct {
		testName string
		runner   *oscommands.FakeCmdObjRunner
		test     func(error)
	}

	scenarios := []scenario{
		{
			testName: "valid case",
			runner: oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"clean", "-fd"}, "", nil),
			test: func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})
			s.test(instance.RemoveUntrackedFiles())
			s.runner.CheckForMissingCalls()
		})
	}
}

func TestWorkingTreeResetHard(t *testing.T) {
	type scenario struct {
		testName string
		ref      string
		runner   *oscommands.FakeCmdObjRunner
		test     func(error)
	}

	scenarios := []scenario{
		{
			"valid case",
			"HEAD",
			oscommands.NewFakeRunner(t).
				ExpectGitArgs([]string{"reset", "--hard", "HEAD"}, "", nil),
			func(err error) {
				assert.NoError(t, err)
			},
		},
	}

	for _, s := range scenarios {
		t.Run(s.testName, func(t *testing.T) {
			instance := buildWorkingTreeCommands(commonDeps{runner: s.runner})
			s.test(instance.ResetHard(s.ref))
		})
	}
}
