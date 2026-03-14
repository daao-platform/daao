/**
 * MarkdownPreview Component
 * 
 * React Markdown renderer for displaying Markdown content
 * with syntax highlighting and GitHub-flavored Markdown support.
 */

import React, { useMemo } from 'react';

// Simple Markdown parser (can be replaced with react-markdown if needed)
// This implementation provides basic GFM support

interface MarkdownPreviewProps {
  content: string;
  className?: string;
  enableSyntaxHighlighting?: boolean;
  enableHeadingIds?: boolean;
  onHeadingClick?: (id: string) => void;
  baseUrl?: string; // For resolving relative links/images
}

// Simple inline code pattern
const inlineCodePattern = /`([^`]+)`/g;
// Code block pattern
const codeBlockPattern = /```(\w*)\n([\s\S]*?)```/g;
// Heading pattern
const headingPattern = /^(#{1,6})\s+(.+)$/gm;
// Bold pattern
const boldPattern = /\*\*([^*]+)\*\*/g;
// Italic pattern
const boldItalicPattern = /\*\*\*([^*]+)\*\*\*/g;
const italicPattern = /\*([^*]+)\*/g;
// Strikethrough pattern
const strikethroughPattern = /~~([^~]+)~~/g;
// Link pattern
const linkPattern = /\[([^\]]+)\]\(([^)]+)\)/g;
// Image pattern
const imagePattern = /!\[([^\]]*)\]\(([^)]+)\)/g;
// Blockquote pattern
const blockquotePattern = /^>\s+(.+)$/gm;
// Unordered list pattern
const unorderedListPattern = /^[-*+]\s+(.+)$/gm;
// Ordered list pattern
const orderedListPattern = /^\d+\.\s+(.+)$/gm;
// Horizontal rule pattern
const hrPattern = /^---$/gm;
// Table pattern
const tablePattern = /^\|(.+)\|\n\|[-:\s|]+\|\n((?:\|.+\|\n?)+)/gm;

/**
 * Parse and render Markdown content
 */
function parseMarkdown(content: string, options: {
  enableHeadingIds?: boolean;
  onHeadingClick?: (id: string) => void;
  baseUrl?: string;
}): React.ReactNode[] {
  const lines = content.split('\n');
  const elements: React.ReactNode[] = [];
  let i = 0;
  let inCodeBlock = false;
  let codeBlockContent = '';
  let codeBlockLanguage = '';
  let inBlockquote = false;
  let blockquoteLines: string[] = [];
  let inList = false;
  let listItems: string[] = [];
  let listType: 'ul' | 'ol' = 'ul';

  const processInline = (text: string): React.ReactNode => {
    const parts: React.ReactNode[] = [];
    let lastIndex = 0;
    let match;
    
    // Reset regex lastIndex
    inlineCodePattern.lastIndex = 0;
    linkPattern.lastIndex = 0;
    imagePattern.lastIndex = 0;
    boldItalicPattern.lastIndex = 0;
    boldPattern.lastIndex = 0;
    italicPattern.lastIndex = 0;
    strikethroughPattern.lastIndex = 0;

    // Process inline elements in order
    const combinedPattern = /(`[^`]+`)|(\*\*\*[^*]+\*\*\*)|(\*\*[^*]+\*\*)|(\*[^*]+\*)|(~~[^~]+~~)|(!\[[^\]]*\]\([^)]+\))|(\[[^\]]+\]\([^)]+\))/g;
    
    let inlineResult: string = text;
    const segments: { type: string; content: string; start: number; end: number }[] = [];
    
    while ((match = combinedPattern.exec(text)) !== null) {
      segments.push({
        type: match[0].startsWith('`') ? 'code' :
              match[0].startsWith('***') ? 'bolditalic' :
              match[0].startsWith('**') ? 'bold' :
              match[0].startsWith('*') ? 'italic' :
              match[0].startsWith('~~') ? 'strike' :
              match[0].startsWith('![') ? 'image' :
              match[0].startsWith('[') ? 'link' : 'text',
        content: match[0],
        start: match.index,
        end: match.index + match[0].length,
      });
    }
    
    segments.sort((a, b) => a.start - b.start);
    
    let currentIndex = 0;
    for (const seg of segments) {
      if (seg.start > currentIndex) {
        parts.push(text.slice(currentIndex, seg.start));
      }
      
      switch (seg.type) {
        case 'code':
          parts.push(<code key={seg.start} className="inline-code">{seg.content.slice(1, -1)}</code>);
          break;
        case 'bolditalic':
          parts.push(<strong key={seg.start}><em>{seg.content.slice(3, -3)}</em></strong>);
          break;
        case 'bold':
          parts.push(<strong key={seg.start}>{seg.content.slice(2, -2)}</strong>);
          break;
        case 'italic':
          parts.push(<em key={seg.start}>{seg.content.slice(1, -1)}</em>);
          break;
        case 'strike':
          parts.push(<del key={seg.start}>{seg.content.slice(2, -2)}</del>);
          break;
        case 'image':
          const imgMatch = seg.content.match(/!\[([^\]]*)\]\(([^)]+)\)/);
          if (imgMatch) {
            parts.push(<img key={seg.start} src={imgMatch[2]} alt={imgMatch[1] || ''} className="md-image" />);
          }
          break;
        case 'link':
          const linkMatch = seg.content.match(/\[([^\]]+)\]\(([^)]+)\)/);
          if (linkMatch) {
            parts.push(<a key={seg.start} href={linkMatch[2]} className="md-link" target="_blank" rel="noopener noreferrer">{linkMatch[1]}</a>);
          }
          break;
      }
      
      currentIndex = seg.end;
    }
    
    if (currentIndex < text.length) {
      parts.push(text.slice(currentIndex));
    }
    
    return parts.length > 0 ? <>{parts}</> : text;
  };

  const generateId = (text: string): string => {
    return text.toLowerCase().replace(/[^\w\s-]/g, '').replace(/\s+/g, '-');
  };

  while (i < lines.length) {
    const line = lines[i];
    
    // Code block handling
    if (line.startsWith('```')) {
      if (!inCodeBlock) {
        inCodeBlock = true;
        codeBlockLanguage = line.slice(3).trim();
        codeBlockContent = '';
      } else {
        elements.push(
          <pre key={`code-${i}`} className="code-block">
            <code className={`language-${codeBlockLanguage}`}>
              {codeBlockContent}
            </code>
          </pre>
        );
        inCodeBlock = false;
      }
      i++;
      continue;
    }
    
    if (inCodeBlock) {
      codeBlockContent += (codeBlockContent ? '\n' : '') + line;
      i++;
      continue;
    }
    
    // Blockquote handling
    if (line.startsWith('>')) {
      inBlockquote = true;
      blockquoteLines.push(line.slice(1).trim());
      i++;
      continue;
    } else if (inBlockquote) {
      elements.push(
        <blockquote key={`bq-${i}`} className="md-blockquote">
          {blockquoteLines.map((l, idx) => (
            <p key={idx}>{processInline(l)}</p>
          ))}
        </blockquote>
      );
      inBlockquote = false;
      blockquoteLines = [];
    }
    
    // List handling
    if (unorderedListPattern.test(line)) {
      if (!inList || listType !== 'ul') {
        if (inList) {
          elements.push(listType === 'ul' 
            ? <ul key={`ul-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ul>
            : <ol key={`ol-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ol>
          );
        }
        listItems = [];
        listType = 'ul';
      }
      inList = true;
      listItems.push(line.replace(unorderedListPattern, '$1'));
      i++;
      continue;
    } else if (orderedListPattern.test(line)) {
      if (!inList || listType !== 'ol') {
        if (inList) {
          elements.push(listType === 'ul' 
            ? <ul key={`ul-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ul>
            : <ol key={`ol-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ol>
          );
        }
        listItems = [];
        listType = 'ol';
      }
      inList = true;
      listItems.push(line.replace(orderedListPattern, '$1'));
      i++;
      continue;
    } else if (inList) {
      elements.push(listType === 'ul' 
        ? <ul key={`ul-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ul>
        : <ol key={`ol-${i}`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ol>
      );
      inList = false;
      listItems = [];
    }
    
    // Empty line
    if (line.trim() === '') {
      i++;
      continue;
    }
    
    // Heading handling
    const headingMatch = line.match(/^(#{1,6})\s+(.+)$/);
    if (headingMatch) {
      const level = headingMatch[1].length;
      const text = headingMatch[2];
      const id = generateId(text);
      
      elements.push(
        <div 
          key={`h-${i}`} 
          className={`md-heading md-h${level}`}
          id={id}
        >
          {processInline(text)}
        </div>
      );
      i++;
      continue;
    }
    
    // Horizontal rule
    if (hrPattern.test(line)) {
      elements.push(<hr key={`hr-${i}`} className="md-hr" />);
      i++;
      continue;
    }
    
    // Regular paragraph
    elements.push(
      <p key={`p-${i}`} className="md-paragraph">
        {processInline(line)}
      </p>
    );
    i++;
  }
  
  // Handle trailing blockquote
  if (inBlockquote && blockquoteLines.length > 0) {
    elements.push(
      <blockquote key={`bq-end`} className="md-blockquote">
        {blockquoteLines.map((l, idx) => (
          <p key={idx}>{processInline(l)}</p>
        ))}
      </blockquote>
    );
  }
  
  // Handle trailing list
  if (inList && listItems.length > 0) {
    elements.push(listType === 'ul' 
      ? <ul key={`ul-end`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ul>
      : <ol key={`ol-end`}>{listItems.map((item, idx) => <li key={idx}>{processInline(item)}</li>)}</ol>
    );
  }

  return elements;
}

/**
 * MarkdownPreview component
 * 
 * Renders Markdown content with GitHub-flavored Markdown support.
 * Includes syntax highlighting for code blocks when enabled.
 */
export const MarkdownPreview: React.FC<MarkdownPreviewProps> = ({
  content,
  className = '',
  enableSyntaxHighlighting = true,
  enableHeadingIds = true,
  onHeadingClick,
  baseUrl,
}) => {
  const renderedContent = useMemo(() => {
    if (!content) {
      return [];
    }
    return parseMarkdown(content, {
      enableHeadingIds,
      onHeadingClick,
      baseUrl,
    });
  }, [content, enableHeadingIds, onHeadingClick, baseUrl]);

  return (
    <div className={`markdown-preview ${className}`}>
      {renderedContent.length > 0 ? (
        renderedContent
      ) : (
        <span className="markdown-empty">No content</span>
      )}
    </div>
  );
};

/**
 * Create a MarkdownPreview component from payload data
 */
export function createMarkdownPreviewProps(payload: Record<string, any>): MarkdownPreviewProps {
  return {
    content: payload.content || payload.markdown || payload.md || '',
    className: payload.className || '',
    enableSyntaxHighlighting: payload.enableSyntaxHighlighting !== false,
    enableHeadingIds: payload.enableHeadingIds !== false,
    baseUrl: payload.baseUrl,
  };
}

export default MarkdownPreview;
