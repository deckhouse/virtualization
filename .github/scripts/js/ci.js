// Copyright 2022 Flant JSC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//@ts-check

/* 
 * this file contains only 3 functions from the original ci.js: 
 * extractCommandFromComment, reactToComment, startWorkflow.
 * original ci.js file can be found here:
 * https://github.com/deckhouse/deckhouse/blob/main/.github/scripts/js/ci.js
*\

/*
 * Extract argv slash command array from comment.
 *
 * @param {string} comment - A comment body.
 * @returns {object}
 */

const { dumpError } = require('./error');
const extractCommandFromComment = (comment) => {
	// Split comment to lines.
	const lines = comment.split(/\r\n|\n|\r/).filter(l => l.startsWith('/'));
	if (lines.length < 1) {
	  return {'err': 'first line is not a slash command'}
	}
  
	// Search for user command in the first line of the comment.
	// User command is a command and a tag name.
	const argv = lines[0].split(/\s+/);
  
	if ( ! /^\/[a-z\d_\-\/.,]+$/.test(argv[0])) {
	  return {'err': 'not a slash command in the first line'};
	}
  
	return {argv, lines}
};
  
module.exports.extractCommandFromComment = extractCommandFromComment;

/**
 * Set reaction to issue comment.
 *
 * @param {object} inputs
 * @param {object} inputs.github - A pre-authenticated octokit/rest.js client with pagination plugins.
 * @param {object} inputs.context - An object containing the context of the workflow run.
 * @param {object} inputs.comment_id - ID of the issue comment.
 * @param {object} inputs.content - Reaction type: (+1, -1, rocket, confused, ...).
 * @returns {Promise<void|*>}
 */
const reactToComment = async ({github, context, comment_id, content}) => {
	return await github.rest.reactions.createForIssueComment({
	  owner: context.repo.owner,
	  repo: context.repo.repo,
	  comment_id,
	  content,
	});
};
module.exports.reactToComment = reactToComment;
  
/**
 * Start workflow using workflow_dispatch event.
 *
 * @param {object} args
 * @param {object} args.github - A pre-authenticated octokit/rest.js client with pagination plugins.
 * @param {object} args.context - An object containing the context of the workflow run.
 * @param {object} args.core - A reference to the '@actions/core' package.
 * @param {object} args.workflow_id - A name of the workflow YAML file.
 * @param {object} args.ref - A Git ref.
 * @param {object} args.inputs - Inputs for the workflow_dispatch event.
 * @returns {Promise<void>}
 */
const startWorkflow = async ({ github, context, core, workflow_id, ref, inputs }) => {
	core.info(`Start workflow '${workflow_id}' using ref '${ref}' and inputs ${JSON.stringify(inputs)}.`);
  
	let response = null
	try {
	  response = await github.rest.actions.createWorkflowDispatch({
		owner: context.repo.owner,
		repo: context.repo.repo,
		workflow_id,
		ref,
		inputs: inputs || {},
	  });
	} catch(error) {
	  return core.setFailed(`Error triggering workflow_dispatch event: ${dumpError(error)}`)
	}
  
	core.debug(`status: ${response.status}`);
	core.debug(`workflow dispatch response: ${JSON.stringify(response)}`);
  
	if (response.status !== 204) {
	  return core.setFailed(`Error triggering workflow_dispatch event for '${workflow_id}'. createWorkflowDispatch response: ${JSON.stringify(response)}`);
	}
	return core.info(`Workflow '${workflow_id}' started successfully`);
};
module.exports.startWorkflow = startWorkflow;