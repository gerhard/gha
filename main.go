// Manage Github Actions configurations with Dagger
//
// Daggerizing your CI makes your YAML configurations smaller, but they still exist,
// and they're still a pain to maintain by hand.
//
// This module aims to finish the job, by letting you generate your remaining
// YAML configuration from a Dagger pipeline, written in your favorite language.
package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/shykes/gha/internal/dagger"
)

var (
	// List of keys available as '${{github.KEY}}'
	// See https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/contexts#github-context
	githubContextKeys = []string{
		// The name of the action currently running, or the id of a step. GitHub removes special characters, and uses the name __run when the current step runs a script without an id. If you use the same action more than once in the same job, the name will include a suffix with the sequence number with underscore before it. For example, the first script you run will have the name __run, and the second script will be named __run_2. Similarly, the second invocation of actions/checkout will be actionscheckout2.
		"action",
		// The path where an action is located. This property is only supported in composite actions. You can use this path to access files located in the same repository as the action, for example by changing directories to the path:  cd ${{ github.action_path }} .
		"action_path",
		// For a step executing an action, this is the ref of the action being executed. For example, v2. Do not use in the run keyword. To make this context work with composite actions, reference it within the env context of the composite action.
		"action_ref",
		// For a step executing an action, this is the owner and repository name of the action. For example, actions/checkout. Do not use in the run keyword. To make this context work with composite actions, reference it within the env context of the composite action.
		"action_repository",
		// For a composite action, the current result of the composite action.
		"action_status",
		// The username of the user that triggered the initial workflow run. If the workflow run is a re-run, this value may differ from github.triggering_actor. Any workflow re-runs will use the privileges of github.actor, even if the actor initiating the re-run (github.triggering_actor) has different privileges.
		"actor",
		// The account ID of the person or app that triggered the initial workflow run. For example, 1234567. Note that this is different from the actor username.
		"actor_id",
		// The URL of the GitHub REST API.
		"api_url",
		// The base_ref or target branch of the pull request in a workflow run. This property is only available when the event that triggers a workflow run is either pull_request or pull_request_target.
		"base_ref",
		// Path on the runner to the file that sets environment variables from workflow commands. This file is unique to the current step and is a different file for each step in a job. For more information, see "Workflow commands for GitHub Actions."
		"env",
		// The name of the event that triggered the workflow run.
		"event_name",
		// The path to the file on the runner that contains the full event webhook payload.
		"event_path",
		// The URL of the GitHub GraphQL API.
		"graphql_url",
		// The head_ref or source branch of the pull request in a workflow run. This property is only available when the event that triggers a workflow run is either pull_request or pull_request_target.
		"head_ref",
		// The job_id of the current job. Note: This context property is set by the Actions runner, and is only available within the execution steps of a job. Otherwise, the value of this property will be null.
		"job",
		// Path on the runner to the file that sets system PATH variables from workflow commands. This file is unique to the current step and is a different file for each step in a job. For more information, see "Workflow commands for GitHub Actions."
		"path",
		// The fully-formed ref of the branch or tag that triggered the workflow run. For workflows triggered by push, this is the branch or tag ref that was pushed. For workflows triggered by pull_request, this is the pull request merge branch. For workflows triggered by release, this is the release tag created. For other triggers, this is the branch or tag ref that triggered the workflow run. This is only set if a branch or tag is available for the event type. The ref given is fully-formed, meaning that for branches the format is refs/heads/<branch_name>, for pull requests it is refs/pull/<pr_number>/merge, and for tags it is refs/tags/<tag_name>. For example, refs/heads/feature-branch-1.
		"ref",
		// The short ref name of the branch or tag that triggered the workflow run. This value matches the branch or tag name shown on GitHub. For example, feature-branch-1. For pull requests, the format is <pr_number>/merge.
		"ref_name",
		// true if branch protections or rulesets are configured for the ref that triggered the workflow run.
		"ref_protected",
		// The type of ref that triggered the workflow run. Valid values are branch or tag.
		"ref_type",
		// The owner and repository name. For example, octocat/Hello-World.
		"repository",
		// The ID of the repository. For example, 123456789. Note that this is different from the repository name.
		"repository_id",
		// The repository owner's username. For example, octocat.
		"repository_owner",
		// The repository owner's account ID. For example, 1234567. Note that this is different from the owner's name.
		"repository_owner_id",
		// The Git URL to the repository. For example, git://github.com/octocat/hello-world.git.
		"repositoryUrl",
		// The number of days that workflow run logs and artifacts are kept.
		"retention_days",
		// A unique number for each workflow run within a repository. This number does not change if you re-run the workflow run.
		"run_id",
		// A unique number for each run of a particular workflow in a repository. This number begins at 1 for the workflow's first run, and increments with each new run. This number does not change if you re-run the workflow run.
		"run_number",
		// A unique number for each attempt of a particular workflow run in a repository. This number begins at 1 for the workflow run's first attempt, and increments with each re-run.
		"run_attempt",
		// The source of a secret used in a workflow. Possible values are None, Actions, Codespaces, or Dependabot.
		"secret_source",
		// The URL of the GitHub server. For example: https://github.com.
		"server_url",
		// The commit SHA that triggered the workflow. The value of this commit SHA depends on the event that triggered the workflow. For more information, see "Events that trigger workflows." For example, ffac537e6cbbf934b08745a378932722df287a53.
		"sha",
		// A token to authenticate on behalf of the GitHub App installed on your repository. This is functionally equivalent to the GITHUB_TOKEN secret. For more information, see "Automatic token authentication." Note: This context property is set by the Actions runner, and is only available within the execution steps of a job. Otherwise, the value of this property will be null.
		"token",
		// The username of the user that initiated the workflow run. If the workflow run is a re-run, this value may differ from github.actor. Any workflow re-runs will use the privileges of github.actor, even if the actor initiating the re-run (github.triggering_actor) has different privileges.
		"triggering_actor",
		// The name of the workflow. If the workflow file doesn't specify a name, the value of this property is the full path of the workflow file in the repository.
		"workflow",
		// The ref path to the workflow. For example, octocat/hello-world/.github/workflows/my-workflow.yml@refs/heads/my_branch.
		"workflow_ref",
		// The commit SHA for the workflow file.
		"workflow_sha",
		// The default working directory on the runner for steps, and the default location of your repository when using the checkout action.
		"workspace",
	}
)

func New(
	// Disable sending traces to Dagger Cloud
	// +optional
	noTraces bool,
	// Public Dagger Cloud token, for open-source projects. DO NOT PASS YOUR PRIVATE DAGGER CLOUD TOKEN!
	// This is for a special "public" token which can safely be shared publicly.
	// To get one, contact support@dagger.io
	// +optional
	publicToken string,
	// Dagger version to run in the Github Actions pipelines
	// +optional
	// +default="latest"
	daggerVersion string,
	// Explicitly stop the Dagger Engine after completing the pipeline
	// +optional
	stopEngine bool,
	// Encode all files as JSON (which is also valid YAML)
	// +optional
	asJson bool,
	// Configure a default runner for all workflows
	// See https://docs.github.com/en/actions/hosting-your-own-runners/managing-self-hosted-runners/using-self-hosted-runners-in-a-workflow
	// +optional
	// +default="ubuntu-latest"
	runner string,
) *Gha {
	return &Gha{Settings: Settings{
		PublicToken:   publicToken,
		NoTraces:      noTraces,
		DaggerVersion: daggerVersion,
		StopEngine:    stopEngine,
		AsJson:        asJson,
		Runner:        runner,
	}}
}

type Gha struct {
	// +private
	PushTriggers []PushTrigger
	// +private
	PullRequestTriggers []PullRequestTrigger
	// +private
	DispatchTriggers []DispatchTrigger
	// +private
	IssueCommentTriggers []IssueCommentTrigger
	// +private
	Settings Settings
}

type Settings struct {
	// +private
	PublicToken string
	// +private
	DaggerVersion string
	// +private
	NoTraces bool
	// +private
	StopEngine bool
	// +private
	AsJson bool
	// +private
	Runner string
}

func (m *Gha) Check(ctx context.Context, repo *dagger.Directory) (*Gha, error) {
	for _, t := range m.PushTriggers {
		if err := t.Pipeline.Check(ctx, repo); err != nil {
			return m, err
		}
	}
	for _, t := range m.PullRequestTriggers {
		if err := t.Pipeline.Check(ctx, repo); err != nil {
			return m, err
		}
	}
	for _, t := range m.DispatchTriggers {
		if err := t.Pipeline.Check(ctx, repo); err != nil {
			return m, err
		}
	}
	for _, t := range m.IssueCommentTriggers {
		if err := t.Pipeline.Check(ctx, repo); err != nil {
			return m, err
		}
	}
	return m, nil
}

// Generate a github config directory, usable as an overlay on the repository root
func (m *Gha) Config(
	// Prefix to use for generated workflow filenames
	// +optional
	prefix string,
) *dagger.Directory {
	dir := dag.Directory()
	for i, t := range m.PushTriggers {
		filename := fmt.Sprintf("%spush-%d.yml", prefix, i+1)
		dir = dir.WithDirectory(".", t.Config(filename, m.Settings.AsJson))
	}
	for i, t := range m.PullRequestTriggers {
		filename := fmt.Sprintf("%spr-%d.yml", prefix, i+1)
		dir = dir.WithDirectory(".", t.Config(filename, m.Settings.AsJson))
	}
	for i, t := range m.DispatchTriggers {
		filename := fmt.Sprintf("%sdispatch-%d.yml", prefix, i+1)
		dir = dir.WithDirectory(".", t.Config(filename, m.Settings.AsJson))
	}
	for i, t := range m.IssueCommentTriggers {
		filename := fmt.Sprintf("%sissue-comment-%d.yml", prefix, i+1)
		dir = dir.WithDirectory(".", t.Config(filename, m.Settings.AsJson))
	}
	return dir
}

func (m *Gha) pipeline(
	// The Dagger command to execute
	// Example 'build --source=.'
	command string,
	module string,
	runner string,
	secrets []string,
	sparseCheckout []string,
) Pipeline {
	p := Pipeline{
		Command:        command,
		Module:         module,
		Secrets:        secrets,
		SparseCheckout: sparseCheckout,
		Settings:       m.Settings,
	}
	if runner != "" {
		p.Settings.Runner = runner
	}
	return p
}

// A Dagger pipeline to be called from a Github Actions configuration
type Pipeline struct {
	// +private
	Module string
	// +private
	Command string
	// +private
	Secrets []string
	// +private
	SparseCheckout []string
	// +private
	Settings Settings
}

func (p *Pipeline) Name() string {
	return strings.SplitN(p.Command, " ", 2)[0]
}

func (p *Pipeline) checkSecretNames() error {
	// check if the secret name contains only alphanumeric characters and underscores.
	validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	for _, secretName := range p.Secrets {
		if !validName.MatchString(secretName) {
			return errors.New("invalid secret name: '" + secretName + "' must contain only alphanumeric characters and underscores")
		}
	}
	return nil
}

func (p *Pipeline) checkCommandAndModule(ctx context.Context, repo *dagger.Directory) error {
	script := "dagger call"
	if p.Module != "" {
		script = script + " -m '" + p.Module + "' "
	}
	script = script + p.Command + " --help"
	_, err := dag.
		Wolfi().
		Container(dagger.WolfiContainerOpts{
			Packages: []string{"dagger", "bash"},
		}).
		WithMountedDirectory("/src", repo).
		WithWorkdir("/src").
		WithExec(
			[]string{"bash", "-c", script},
			dagger.ContainerWithExecOpts{ExperimentalPrivilegedNesting: true},
		).
		Sync(ctx)
	return err
}

// Check that the pipeline is valid, in a best effort way
func (p *Pipeline) Check(
	ctx context.Context,
	// +defaultPath="/"
	repo *dagger.Directory,
) error {
	if err := p.checkSecretNames(); err != nil {
		return err
	}
	if err := p.checkCommandAndModule(ctx, repo); err != nil {
		return err
	}
	return nil
}

// Generate a GHA workflow from a Dagger pipeline definition.
// The workflow will have no triggers, they should be filled separately.
func (p *Pipeline) asWorkflow() Workflow {
	steps := []JobStep{
		p.checkoutStep(),
		p.installDaggerStep(),
		p.warmEngineStep(),
		p.callDaggerStep(),
	}
	if p.Settings.StopEngine {
		steps = append(steps, p.stopEngineStep())
	}
	return Workflow{
		Name: p.Command,
		On:   WorkflowTriggers{}, // Triggers intentionally left blank
		Jobs: map[string]Job{
			"dagger": Job{
				RunsOn: p.Settings.Runner,
				Steps:  steps,
				Outputs: map[string]string{
					"stdout": "${{ steps.exec.outputs.stdout }}",
					"stderr": "${{ steps.exec.outputs.stderr }}",
				},
			},
		},
	}
}

func (p *Pipeline) checkoutStep() JobStep {
	step := JobStep{
		Name: "Checkout",
		Uses: "actions/checkout@v4",
	}
	if p.SparseCheckout != nil {
		// Include common dagger paths in the checkout, to make
		// sure local modules work by default
		// FIXME: this is only a guess, we need the 'source' field of dagger.json
		//  to be sure.
		sparseCheckout := append(p.SparseCheckout, "dagger.json", ".dagger", "dagger", "ci")
		step.With = map[string]string{
			"sparse-checkout": strings.Join(sparseCheckout, "\n"),
		}
	}
	return step
}

func (p *Pipeline) warmEngineStep() JobStep {
	return p.bashStep("warm-engine", nil)
}

func (p *Pipeline) installDaggerStep() JobStep {
	return p.bashStep("install-dagger", map[string]string{
		"DAGGER_VERSION": p.Settings.DaggerVersion,
	})
}

func (p *Pipeline) callDaggerStep() JobStep {
	env := map[string]string{}
	// Inject dagger command
	env["COMMAND"] = "dagger call -q " + p.Command
	// Inject user-defined secrets
	for _, secretName := range p.Secrets {
		env[secretName] = fmt.Sprintf("${{ secrets.%s }}", secretName)
	}
	// Inject module name
	if p.Module != "" {
		env["DAGGER_MODULE"] = p.Module
	}
	// Inject Dagger Cloud token
	if !p.Settings.NoTraces {
		if p.Settings.PublicToken != "" {
			env["DAGGER_CLOUD_TOKEN"] = p.Settings.PublicToken
			// For backwards compatibility with older engines
			env["_EXPERIMENTAL_DAGGER_CLOUD_TOKEN"] = p.Settings.PublicToken
		} else {
			env["DAGGER_CLOUD_TOKEN"] = "${{ secrets.DAGGER_CLOUD_TOKEN }}"
			// For backwards compatibility with older engines
			env["_EXPERIMENTAL_DAGGER_CLOUD_TOKEN"] = "${{ secrets.DAGGER_CLOUD_TOKEN }}"
		}
	}
	// Inject Github context keys
	// github.ref becomes $GITHUB_REF, etc.
	for _, key := range githubContextKeys {
		env["GITHUB_"+strings.ToUpper(key)] = fmt.Sprintf("${{ github.%s }}", key)
	}
	return p.bashStep("exec", env)
}

func (p *Pipeline) stopEngineStep() JobStep {
	return p.bashStep("scripts/stop-engine.sh", nil)
}

// Return a github actions step which executes the script embedded at scripts/<filename>.sh
// The script must be checked in with the module source code.
func (p *Pipeline) bashStep(id string, env map[string]string) JobStep {
	filename := "scripts/" + id + ".sh"
	script, err := dag.
		CurrentModule().
		Source().
		File(filename).
		Contents(context.Background())
	if err != nil {
		// We skip error checking for simplicity
		// (don't want to plumb error checking everywhere)
		panic(err)
	}
	return JobStep{
		Name:  filename,
		ID:    id,
		Shell: "bash",
		Run:   script,
		Env:   env,
	}
}
