package com.congmingpay.android.util

import android.view.View
import android.view.ViewGroup
import androidx.core.graphics.Insets
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.updatePadding

/**
 * 系统栏避让：按分辨率/手势导航动态垫高，避免底栏/按钮画进系统导航栏或状态栏。
 */
object UiInsets {

    /**
     * 给根布局垫 systemBars（状态栏 + 导航栏）。
     * @param consume 是否消费 insets（主窗根布局通常 true）
     */
    fun applySystemBars(root: View, consume: Boolean = true) {
        val initialLeft = root.paddingLeft
        val initialTop = root.paddingTop
        val initialRight = root.paddingRight
        val initialBottom = root.paddingBottom
        ViewCompat.setOnApplyWindowInsetsListener(root) { v, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
            v.updatePadding(
                left = initialLeft + bars.left,
                top = initialTop + bars.top,
                right = initialRight + bars.right,
                bottom = initialBottom + bars.bottom
            )
            if (consume) WindowInsetsCompat.CONSUMED else insets
        }
        root.requestApplyInsets()
    }

    /** 仅垫底部导航栏（工具条贴底时用） */
    fun applyNavigationBarBottom(view: View) {
        val initialBottom = view.paddingBottom
        ViewCompat.setOnApplyWindowInsetsListener(view) { v, insets ->
            val nav = insets.getInsets(WindowInsetsCompat.Type.navigationBars())
            v.updatePadding(bottom = initialBottom + nav.bottom)
            insets
        }
        view.requestApplyInsets()
    }

    fun systemBars(view: View): Insets {
        val insets = ViewCompat.getRootWindowInsets(view) ?: return Insets.NONE
        return insets.getInsets(WindowInsetsCompat.Type.systemBars())
    }

    /** 是否宽屏/收款机类（最短边 ≥ 600dp 或横屏且宽 ≥ 600dp） */
    fun isWideUi(view: View): Boolean {
        val dm = view.resources.displayMetrics
        val shortest = minOf(dm.widthPixels, dm.heightPixels) / dm.density
        return shortest >= 600f
    }
}
