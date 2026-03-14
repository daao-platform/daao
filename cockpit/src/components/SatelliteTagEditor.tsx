import React, { useState, KeyboardEvent } from 'react';
import './SatelliteTagEditor.css';

interface SatelliteTagEditorProps {
    satelliteId: string;
    tags: string[];
    autoTags?: string[];
    onSave: (tags: string[]) => Promise<void>;
    disabled?: boolean;
}

const MAX_TAGS = 20;
const MAX_TAG_LENGTH = 50;

/**
 * Validates a tag: lowercase alphanumeric + hyphens only
 */
const isValidTag = (tag: string): boolean => {
    return /^[a-z0-9-]+$/.test(tag) && tag.length > 0 && tag.length <= MAX_TAG_LENGTH;
};

export const SatelliteTagEditor: React.FC<SatelliteTagEditorProps> = ({
    satelliteId,
    tags,
    autoTags = [],
    onSave,
    disabled = false,
}) => {
    const [inputValue, setInputValue] = useState('');
    const [error, setError] = useState<string | null>(null);
    const [isSaving, setIsSaving] = useState(false);

    // Combine editable tags and auto-tags for display
    const allTags = [...tags];

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        setInputValue(e.target.value);
        setError(null);
    };

    const handleRemoveTag = (tagToRemove: string) => {
        if (disabled || isSaving) return;
        const newTags = tags.filter((tag) => tag !== tagToRemove);
        onSave(newTags);
    };

    const handleAddTag = async () => {
        if (disabled || isSaving) return;

        const trimmedTag = inputValue.trim().toLowerCase();

        // Validation checks
        if (!trimmedTag) {
            setError('Tag cannot be empty');
            return;
        }

        if (!isValidTag(trimmedTag)) {
            setError('Tags must be lowercase letters, numbers, and hyphens only');
            return;
        }

        if (trimmedTag.length > MAX_TAG_LENGTH) {
            setError(`Tag must be ${MAX_TAG_LENGTH} characters or less`);
            return;
        }

        if (allTags.length >= MAX_TAGS) {
            setError(`Maximum of ${MAX_TAGS} tags allowed`);
            return;
        }

        if (tags.includes(trimmedTag)) {
            setError('Tag already exists');
            return;
        }

        // Add the tag
        const newTags = [...tags, trimmedTag];
        setIsSaving(true);
        setError(null);

        try {
            await onSave(newTags);
            setInputValue('');
        } catch (err) {
            setError('Failed to save tag');
        } finally {
            setIsSaving(false);
        }
    };

    const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            handleAddTag();
        }
    };

    const handleSaveClick = async () => {
        if (disabled || isSaving) return;
        setIsSaving(true);
        try {
            await onSave(tags);
        } catch (err) {
            setError('Failed to save');
        } finally {
            setIsSaving(false);
        }
    };

    return (
        <div className="tag-editor">
            <div className="tag-editor__header">
                <span className="tag-editor__label">Tags</span>
                {disabled && (
                    <span className="tag-editor__enterprise-badge">
                        <svg
                            width="10"
                            height="10"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            strokeWidth="2.5"
                            strokeLinecap="round"
                            strokeLinejoin="round"
                        >
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                        </svg>
                        Enterprise
                    </span>
                )}
            </div>

            <div className="tag-editor__chips">
                {/* Auto-tags (read-only) */}
                {autoTags.map((tag) => (
                    <span key={`auto-${tag}`} className="tag-editor-chip tag-editor-chip--auto">
                        {tag}
                    </span>
                ))}

                {/* Editable tags */}
                {tags.map((tag) => (
                    <span key={tag} className="tag-editor-chip">
                        {tag}
                        {!disabled && (
                            <button
                                className="tag-editor-chip__remove"
                                onClick={() => handleRemoveTag(tag)}
                                disabled={isSaving}
                                aria-label={`Remove ${tag} tag`}
                                type="button"
                            >
                                ×
                            </button>
                        )}
                    </span>
                ))}
            </div>

            {!disabled && (
                <>
                    <div className="tag-editor__input-row">
                        <input
                            type="text"
                            className="tag-editor-input"
                            value={inputValue}
                            onChange={handleInputChange}
                            onKeyDown={handleKeyDown}
                            placeholder="Add a tag..."
                            disabled={isSaving}
                            maxLength={MAX_TAG_LENGTH}
                        />
                        <button
                            className="tag-editor__add-btn"
                            onClick={handleAddTag}
                            disabled={isSaving || !inputValue.trim()}
                            type="button"
                        >
                            Add
                        </button>
                    </div>

                    {error && <div className="tag-editor__error">{error}</div>}
                </>
            )}

            <div className="tag-editor__footer">
                <span className="tag-editor__count">
                    {allTags.length} / {MAX_TAGS} tags
                </span>
                {!disabled && (
                    <button
                        className="tag-editor__save-btn btn btn--primary btn--sm"
                        onClick={handleSaveClick}
                        disabled={isSaving}
                        type="button"
                    >
                        {isSaving ? 'Saving...' : 'Save'}
                    </button>
                )}
            </div>
        </div>
    );
};

export default SatelliteTagEditor;
