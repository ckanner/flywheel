import { useEffect, useState } from 'react';
import { styles } from 'virtual:flywheel-styles';
import type { FlywheelSnapshot, FlywheelStore } from '../store.ts';
import type { Translate } from '../types.ts';

type TaskIntent = 'build' | 'investigate' | 'quick';

function slugifyGoal(goal: string, intent: TaskIntent): string {
  const ascii = goal
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 28);
  const stamp = new Date().toISOString().slice(0, 10);
  return ascii ? `${ascii}-${stamp}` : `${intent}-${stamp}`;
}

export function CreateTaskModal({
  snap,
  store,
  t,
}: {
  snap: FlywheelSnapshot;
  store: FlywheelStore;
  t: Translate;
}) {
  const [intent, setIntent] = useState<TaskIntent>('build');
  const [cwd, setCwd] = useState(snap.selectedCwd || '');
  const [slug, setSlug] = useState('');
  const [goal, setGoal] = useState('');
  const [mode, setMode] = useState<'auto' | 'full' | 'quick'>('auto');
  const [readPaths, setReadPaths] = useState('');
  const [selectedImages, setSelectedImages] = useState<string[]>([]);

  useEffect(() => {
    if (snap.createModalOpen && !cwd && snap.selectedCwd) {
      setCwd(snap.selectedCwd);
    }
  }, [snap.createModalOpen, snap.selectedCwd, cwd]);

  if (!snap.createModalOpen) return null;

  const toggleImage = (path: string) => {
    setSelectedImages(prev => {
      if (prev.includes(path)) return prev.filter(p => p !== path);
      if (prev.length >= 4) return prev;
      return [...prev, path];
    });
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!cwd.trim() || !goal.trim()) return;
    const finalSlug = slug.trim() || slugifyGoal(goal, intent);
    if (intent === 'investigate') {
      const paths = readPaths
        .split(',')
        .map(s => s.trim())
        .filter(Boolean);
      const res = await store.createInvestigationTask({
        cwd: cwd.trim(),
        slug: finalSlug,
        goal: goal.trim(),
        readPaths: paths,
      });
      if (res) store.setCreateModalOpen(false);
    } else {
      const res = await store.createBuildTask({
        cwd: cwd.trim(),
        slug: finalSlug,
        goal: goal.trim(),
        mode: intent === 'quick' ? 'quick' : mode,
        images: selectedImages,
      });
      if (res) store.setCreateModalOpen(false);
    }
  };

  return (
    <div
      className={styles.modalBackdrop}
      role="dialog"
      aria-modal="true"
      aria-label={t('create.title')}
      onClick={e => {
        if (e.target === e.currentTarget) store.setCreateModalOpen(false);
      }}
    >
      <form className={styles.modal} onSubmit={handleSubmit} data-testid="flywheel-create-modal">
        <div className={styles.cardHeader}>
          <div>
            <h3 className={styles.cardTitle} style={{ fontSize: 15 }}>{t('create.title')}</h3>
            <div style={{ fontSize: 12, color: 'var(--fw-text-muted)' }}>{t('create.subtitle')}</div>
          </div>
          <button type="button" className={`${styles.btn} ${styles.btnSm}`} onClick={() => store.setCreateModalOpen(false)}>
            {t('inspector.close')}
          </button>
        </div>

        {/* 3 Intent Cards */}
        <div className={styles.intentGrid}>
          {(
            [
              { id: 'build', title: t('create.intent.build'), desc: t('create.intent.buildDesc') },
              { id: 'investigate', title: t('create.intent.investigate'), desc: t('create.intent.investigateDesc') },
              { id: 'quick', title: t('create.intent.quick'), desc: t('create.intent.quickDesc') },
            ] as const
          ).map(card => (
            <button
              key={card.id}
              type="button"
              data-intent={card.id}
              className={`${styles.intentCard} ${intent === card.id ? styles.intentCardActive : ''}`}
              onClick={() => setIntent(card.id)}
            >
              <strong style={{ fontSize: 13 }}>{card.title}</strong>
              <span style={{ fontSize: 11.5, color: 'var(--fw-text-muted)' }}>{card.desc}</span>
            </button>
          ))}
        </div>

        <div className={styles.field}>
          <label className={styles.fieldLabel}>{t('create.cwd')}</label>
          <input
            className={styles.input}
            value={cwd}
            onChange={e => setCwd(e.target.value)}
            placeholder="/absolute/path/to/repo"
            required
          />
        </div>

        <div className={styles.field}>
          <label className={styles.fieldLabel}>{t('create.goal')}</label>
          <textarea
            className={styles.textarea}
            value={goal}
            onChange={e => setGoal(e.target.value)}
            placeholder={t('create.goalPlaceholder')}
            required
          />
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
          <div className={styles.field}>
            <label className={styles.fieldLabel}>{t('create.slug')}</label>
            <input
              className={styles.input}
              value={slug}
              onChange={e => setSlug(e.target.value)}
              placeholder="auto-generated"
            />
          </div>

          {intent === 'build' && (
            <div className={styles.field}>
              <label className={styles.fieldLabel}>{t('create.mode')}</label>
              <select
                className={styles.select}
                value={mode}
                onChange={e => setMode(e.target.value as 'auto' | 'full' | 'quick')}
              >
                <option value="auto">{t('create.mode.auto')}</option>
                <option value="full">{t('create.mode.full')}</option>
                <option value="quick">{t('create.mode.quick')}</option>
              </select>
            </div>
          )}

          {intent === 'investigate' && (
            <div className={styles.field}>
              <label className={styles.fieldLabel}>{t('create.readPaths')}</label>
              <input
                className={styles.input}
                value={readPaths}
                onChange={e => setReadPaths(e.target.value)}
                placeholder="src/, internal/"
              />
            </div>
          )}
        </div>

        {/* Visual Image Picker for Build / Quick tasks */}
        {intent !== 'investigate' && (
          <div className={styles.card}>
            <div className={styles.cardHeader}>
              <span className={styles.fieldLabel}>
                {t('create.images')} ({selectedImages.length}/4)
              </span>
              <button
                type="button"
                className={`${styles.btn} ${styles.btnSm}`}
                disabled={!cwd.trim()}
                onClick={() => void store.loadRepoImages(cwd.trim())}
              >
                {t('create.imagesLoad')}
              </button>
            </div>
            {snap.repoImages.length === 0 ? (
              <div style={{ fontSize: 12, color: 'var(--fw-text-subtle)' }}>{t('create.imagesEmpty')}</div>
            ) : (
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, maxHeight: 110, overflowY: 'auto' }}>
                {snap.repoImages.map(img => {
                  const active = selectedImages.includes(img.path);
                  return (
                    <button
                      key={img.path}
                      type="button"
                      className={`${styles.btn} ${styles.btnSm} ${active ? styles.btnPrimary : ''}`}
                      onClick={() => toggleImage(img.path)}
                    >
                      {img.path}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        )}

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button type="button" className={styles.btn} onClick={() => store.setCreateModalOpen(false)}>
            {t('design.cancel')}
          </button>
          <button
            type="submit"
            className={`${styles.btn} ${styles.btnPrimary}`}
            disabled={snap.busyAction !== null || !cwd.trim() || !goal.trim()}
          >
            {snap.busyAction ? t('create.submitting') : t('create.submit')}
          </button>
        </div>
      </form>
    </div>
  );
}
