import { Extension, type JSONContent } from '@tiptap/core';
import Link from '@tiptap/extension-link';
import Mention, { type MentionNodeAttrs } from '@tiptap/extension-mention';
import { Markdown } from '@tiptap/markdown';
import StarterKit from '@tiptap/starter-kit';
import type { SuggestionOptions } from '@tiptap/suggestion';

export interface CommentPerson {
  id: string;
  name: string;
  email?: string;
}
export type CommentMentionAttrs = MentionNodeAttrs;
export type CommentSuggestions = Omit<
  SuggestionOptions<CommentPerson, CommentMentionAttrs>,
  'editor'
>;

function plainText(nodes: JSONContent[]): string {
  return nodes.map((node) => node.text || plainText(node.content || [])).join('');
}

// Keep the existing Markdown wire/storage format. An editor mention is an
// atomic inline node, not a profile lookup, HTML blob, or permission grant.
const CommentMention = Mention.extend({
  markdownTokenName: 'commentMention',
  markdownTokenizer: {
    name: 'commentMention',
    level: 'inline',
    start: (source) => source.indexOf('[@'),
    tokenize(source, _tokens, lexer) {
      const match = /^\[@((?:\\.|[^\]\\])*)\]\(mention:([A-Za-z0-9_-]{1,64})\)/.exec(source);
      if (!match) {
        return undefined;
      }
      return {
        type: 'commentMention',
        raw: match[0],
        id: match[2],
        tokens: lexer.inlineTokens(match[1]),
      };
    },
  },
  parseMarkdown(token, helpers) {
    return helpers.createNode('mention', {
      id: token.id,
      label: plainText(helpers.parseInline(token.tokens || [])),
    });
  },
  renderMarkdown(node) {
    const label = String(node.attrs?.label || node.attrs?.id || '')
      .replace(/[\\`*_[\]<>]/g, '\\$&')
      .replace(/[\r\n]/g, ' ');
    return `[@${label}](mention:${node.attrs?.id})`;
  },
});

// Match the backend's no-raw-HTML contract for Markdown block content.
const LiteralHTML = Extension.create({
  name: 'commentLiteralHTML',
  markdownTokenName: 'html',
  parseMarkdown(token, helpers) {
    const text = helpers.createTextNode(token.raw || token.text || '');
    return token.block ? helpers.createNode('paragraph', {}, [text]) : text;
  },
});

const ImageAltText = Extension.create({
  name: 'commentImageAltText',
  markdownTokenName: 'image',
  parseMarkdown(token, helpers) {
    return token.text ? helpers.createTextNode(token.text) : [];
  },
});

export function commentExtensions(suggestion: CommentSuggestions) {
  return [
    StarterKit.configure({ link: false, underline: false, strike: false }),
    Link.configure({
      openOnClick: false,
      autolink: false,
      linkOnPaste: false,
      markdownLinks: true,
    }),
    CommentMention.configure({
      HTMLAttributes: { class: 'comment-mention' },
      deleteTriggerWithBackspace: true,
      suggestion,
    }),
    LiteralHTML,
    ImageAltText,
    Markdown.configure({ markedOptions: { gfm: false, breaks: true } }),
  ];
}
